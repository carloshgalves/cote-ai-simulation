package operationalmetrics

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestF4MetricsPagerAndForeignSchema(t *testing.T) {
	ctx := context.Background()
	causal, sidecar := metricFixture(t)
	original, err := os.ReadFile(causal)
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(filepath.Dir(causal), "immutable.log")
	logBytes := []byte("sealed model-independent log")
	if err := os.WriteFile(log, logBytes, 0600); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if r, err := OpenRecorder(ctx, causal, causal+suffix); err == nil {
			r.Close()
			t.Fatal("metrics recorder opened reserved causal pager filename")
		}
		if _, err := os.Lstat(causal + suffix); !os.IsNotExist(err) {
			t.Fatal("metrics recorder created causal pager companion")
		}
		for _, link := range []func(string, string) error{os.Symlink, os.Link} {
			if err := link(log, sidecar+suffix); err != nil {
				t.Fatal(err)
			}
			if r, err := OpenRecorder(ctx, causal, sidecar); err == nil {
				r.Close()
				t.Fatal("metrics companion aliases immutable log")
			}
			if _, err := os.Lstat(sidecar); !os.IsNotExist(err) {
				t.Fatal("unsafe open created metrics DB")
			}
			if err := os.Remove(sidecar + suffix); err != nil {
				t.Fatal(err)
			}
		}
	}
	// An orphan regular journal is unsafe even when not linked. A
	// fresh metrics open must not truncate or recover it.
	orphan := sidecar + "-journal"
	if err := os.WriteFile(orphan, logBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := OpenRecorder(ctx, causal, sidecar); err == nil {
		r.Close()
		t.Fatal("fresh metrics opened over an orphan journal")
	}
	if got, err := os.ReadFile(orphan); err != nil || !bytes.Equal(got, logBytes) {
		t.Fatal("orphan journal damaged")
	}
	if err := os.Remove(orphan); err != nil {
		t.Fatal(err)
	}
	if r, err := OpenRecorder(ctx, causal, log); err == nil {
		r.Close()
		t.Fatal("metrics accepted unrelated log")
	}
	foreign, err := sql.Open("sqlite", sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.Exec("PRAGMA user_version=1; CREATE TABLE foreign_data(x); INSERT INTO foreign_data VALUES(1)"); err != nil {
		t.Fatal(err)
	}
	if err := foreign.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := OpenRecorder(ctx, causal, sidecar); err == nil {
		r.Close()
		t.Fatal("foreign DB with matching version became operational store")
	}
	if r, err := OpenReader(ctx, causal, sidecar); err == nil {
		r.Close()
		t.Fatal("foreign DB with matching version became operational reader")
	}
	for i, path := range []string{causal, log, sidecar} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, [][]byte{original, logBytes, raw}[i]) {
			t.Fatal("rejected metrics open changed protected bytes")
		}
	}
}

func TestF4MetricsInjectedTriggerAndPathReplacement(t *testing.T) {
	ctx := context.Background()
	causal, sidecar := metricFixture(t)
	r, err := OpenRecorder(ctx, causal, sidecar)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s := metricSample(1, 10)
	if err := r.Record(ctx, s); err != nil {
		t.Fatal(err)
	}
	view, err := OpenReader(ctx, causal, sidecar)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	admin, err := sql.Open("sqlite", sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	if err := r.Record(ctx, metricSample(2, 20)); err == nil {
		t.Fatal("opened recorder ignored changed schema version")
	}
	if got, err := view.Read(ctx, s.Run, 10); err == nil || len(got) != 0 {
		t.Fatal("opened reader ignored changed schema version")
	}
	if _, err := admin.Exec("PRAGMA user_version=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE TRIGGER forged AFTER INSERT ON operational_samples BEGIN DELETE FROM operational_samples; END"); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	if err := admin.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Record(ctx, metricSample(2, 20)); err == nil {
		t.Fatal("metrics recorder ran injected trigger")
	}
	if v, err := view.Read(ctx, s.Run, 10); err == nil || len(v) != 0 {
		t.Fatal("metrics reader accepted foreign trigger")
	}
	after, err := os.ReadFile(sidecar)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("schema rejection changed durable bytes")
	}
	// Simulate a name replacement after both connections are established.
	// Keep the original safe, compare through the already-open capabilities.
	saved := sidecar + ".saved"
	if err := os.Rename(sidecar, saved); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, before, 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Record(ctx, metricSample(3, 30)); err == nil {
		t.Fatal("recorder kept accepting renamed authority")
	}
	if v, err := view.Read(ctx, s.Run, 10); err == nil || len(v) != 0 {
		t.Fatal("reader kept accepting renamed authority")
	}
}
