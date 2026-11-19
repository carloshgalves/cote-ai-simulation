//go:build linux

package commit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/operationalmetrics"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// Capture database bytes, including WAL where present, independently of
// E1's canonical rows. SHM is ephemeral SQLite lock/index coordination,
// not a durable ledger. A quiescent RW anchor keeps existing WAL files
// alive across reader opens/closes, never commits during the experiment.
func f4DurableFiles(t *testing.T, paths ...string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, path := range paths {
		for _, suffix := range []string{"", "-wal", "-journal"} {
			raw, err := os.ReadFile(path + suffix)
			if os.IsNotExist(err) && suffix != "" {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			out[path+suffix] = raw
		}
	}
	return out
}

func TestReferenceF4ConcurrentReadOnly(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, owner := referenceE2Paths(t)
	p, err := RunReferenceD3(ctx, path, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, ReferenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	run := referenceC2Decode32(t, p.RunID)
	anchor, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Close()
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err := referenceD3ActorDispatches(ctx, db, run)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(filepath.Dir(path), "host-ops.db")
	rec, err := operationalmetrics.OpenRecorder(ctx, path, sidecar)
	if err != nil {
		t.Fatal(err)
	}
	sample := operationalmetrics.Sample{ID: [32]byte{1}, Run: run, Stage: "replay", HostID: "test-host", WorkerCalls: 1}
	if err := rec.Record(ctx, sample); err != nil {
		rec.Close()
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	before := referenceE2Capture(t, path, owner)
	// Discover both owner DBs under the private test directory rather than
	// relying on the exact names of the owner provisioning helper.
	files := []string{path, sidecar}
	entries, err := os.ReadDir(filepath.Dir(owner))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".db" {
			files = append(files, filepath.Join(filepath.Dir(owner), entry.Name()))
		}
	}
	if len(files) != 4 {
		t.Fatal("missing independent A and B owner DBs")
	}
	raw := f4DurableFiles(t, files...)
	audit, err := sqlite.OpenPrivilegedObservatory(ctx, path, host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	a, err := sqlite.OpenPOVObservatory(ctx, path, host.Bundle, run, dispatch[0], [2]string{"school", "a"})
	if err != nil {
		audit.Close()
		t.Fatal(err)
	}
	b, err := sqlite.OpenPOVObservatory(ctx, path, host.Bundle, run, dispatch[1], [2]string{"school", "b"})
	if err != nil {
		a.Close()
		audit.Close()
		t.Fatal(err)
	}
	pub, err := sqlite.OpenPublicObservatory(ctx, path, host.Bundle)
	if err != nil {
		b.Close()
		a.Close()
		audit.Close()
		t.Fatal(err)
	}
	metrics, err := operationalmetrics.OpenReader(ctx, path, sidecar)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		metrics.Close()
		pub.Close()
		b.Close()
		a.Close()
		audit.Close()
	})
	projections := []func() (any, error){
		func() (any, error) { return audit.ReadPrivilegedReport(ctx, run) },
		func() (any, error) { return audit.ReadOperationalReport(ctx, run) },
		func() (any, error) { return a.ReadPOV(ctx) },
		func() (any, error) { return b.ReadPOV(ctx) },
		func() (any, error) { return pub.ReadPublic(ctx, run) },
		func() (any, error) { return metrics.Read(ctx, run, 10) },
	}
	expected := make([]any, len(projections))
	for i, read := range projections {
		expected[i], err = read()
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(expected[0].(observatory.PrivilegedReport).Commits) != 9 ||
		len(expected[2].(observatory.POVView).Entries) == 0 || len(expected[3].(observatory.POVView).Entries) == 0 ||
		expected[4].(observatory.PublicView).PublicationStatus != observatory.NoPublicRoute ||
		!reflect.DeepEqual(expected[5], []operationalmetrics.Sample{sample}) {
		t.Fatal("vacuous concurrent baseline")
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for worker := 0; worker < 12; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for n := 0; n < 6; n++ {
				i := (worker + n) % len(projections)
				got, err := projections[i]()
				if err != nil || !reflect.DeepEqual(got, expected[i]) {
					errs <- fmt.Errorf("projection %d unstable: %v", i, err)
					return
				}
			}
		}(worker)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if err := referenceE1Compare(before, referenceE2Capture(t, path, owner)); err != nil {
		t.Fatal("concurrent reads changed authoritative rows/cursors/checkpoints", err)
	}
	for _, closeReader := range []func() error{metrics.Close, pub.Close, b.Close, a.Close, audit.Close} {
		if err := closeReader(); err != nil {
			t.Fatal(err)
		}
	}
	after := f4DurableFiles(t, files...)
	if !reflect.DeepEqual(raw, after) {
		t.Fatal("concurrent queries changed durable database or WAL bytes")
	}
	t.Log("F4 verified 72 concurrent queries, six surfaces, three authoritative DB transcripts and immutable durable file bytes")
}

func TestReferenceF4PathAndPagerControls(t *testing.T) {
	ctx := context.Background()
	_, _, _, run, bundle, path := testFixture(t)
	before := o1WholeDatabaseDigest(t, path)
	q, err := sqlite.OpenObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	dir := filepath.Dir(path)
	for _, tc := range []struct {
		name string
		link func(string, string) error
	}{
		{"symlink", os.Symlink},
		{"hardlink", os.Link},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alias := filepath.Join(dir, "alias-"+tc.name+".db")
			if err := tc.link(path, alias); err != nil {
				t.Fatal(err)
			}
			if r, err := sqlite.OpenObservatory(ctx, alias, bundle); err == nil {
				r.Close()
				t.Fatal("alias opened as read authority")
			}
			if tc.name == "hardlink" {
				if _, err := q.ReadRun(ctx, [32]byte(run)); err == nil {
					t.Fatal("open reader ignored newly introduced hardlink")
				}
			}
			if err := os.Remove(alias); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("parent-symlink", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "linked")
		if err := os.Symlink(dir, parent); err != nil {
			t.Fatal(err)
		}
		if r, err := sqlite.OpenObservatory(ctx, filepath.Join(parent, filepath.Base(path)), bundle); err == nil {
			r.Close()
			t.Fatal("symlink ancestor accepted")
		}
	})
	t.Run("group-write-file", func(t *testing.T) {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0660); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(path, info.Mode().Perm())
		if _, err := q.ReadRun(ctx, [32]byte(run)); err == nil {
			t.Fatal("reader ignored unsafe file rights")
		}
	})
	t.Run("group-write-parent", func(t *testing.T) {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0770); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(dir, info.Mode().Perm())
		if r, err := sqlite.OpenObservatory(ctx, path, bundle); err == nil {
			r.Close()
			t.Fatal("writable ancestor accepted")
		}
	})
	t.Run("pager-link", func(t *testing.T) {
		// The actual WAL is active: a distinct rollback-journal name is
		// unused by the valid run. Even that reserved family must be safe.
		log := filepath.Join(dir, "authority.log")
		secret := []byte("operator immutable log")
		if err := os.WriteFile(log, secret, 0600); err != nil {
			t.Fatal(err)
		}
		pager := path + "-journal"
		if err := os.Symlink(log, pager); err != nil {
			t.Fatal(err)
		}
		if r, err := sqlite.OpenObservatory(ctx, path, bundle); err == nil {
			r.Close()
			t.Fatal("hostile pager link accepted")
		}
		if _, err := q.ReadRun(ctx, [32]byte(run)); err == nil {
			t.Fatal("open reader ignored hostile pager link")
		}
		if err := os.Remove(pager); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(log)
		if err != nil || !bytes.Equal(got, secret) {
			t.Fatal("reader damaged immutable log")
		}
	})
	if _, err := q.ReadRun(ctx, [32]byte(run)); err != nil {
		t.Fatal("restored safe authority not readable", err)
	}
	if got := o1WholeDatabaseDigest(t, path); got != before {
		t.Fatal("path rejection changed actual causal store")
	}
}
