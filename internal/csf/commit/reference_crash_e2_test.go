//go:build linux

package commit

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const e2ExitAfterCommit=86

// This helper runs in a NEW EXECUTABLE INSTANCE, not a goroutine or reused Go
// Coordinator. It receives only on-disk SQLite files, a PUBLIC pinned A1
// manifest, and the immutable RunID/snapshot ID. The source producer's signing
// key, original sealed handles and ApprovedCut never cross into this process.
func TestReferenceE2RecoveryProcessChild(t *testing.T) {
	if os.Getenv("CSF_E2_CHILD")!="strict-recovery" {return}
	ctx:=context.Background()
	decode:=func(name string)[32]byte{
		var out [32]byte
		raw,err:=hex.DecodeString(os.Getenv(name))
		if err!=nil||len(raw)!=32{t.Fatalf("invalid E2 child ID %s",name)}
		copy(out[:],raw)
		return out
	}
	run:=decode("CSF_E2_RUN")
	snapshot:=decode("CSF_E2_SNAPSHOT")
	base,err:=policy.Load(bundlePath)
	if err!=nil{t.Fatal(err)}
	manifest,err:=os.ReadFile(os.Getenv("CSF_E2_MANIFEST"))
	if err!=nil{t.Fatal(err)}
	b,err:=policy.PhysicalExtensionV1(base,manifest)
	if err!=nil{t.Fatal(err)}
	store,err:=sqlite.Open(ctx,os.Getenv("CSF_E2_CAUSAL"))
	if err!=nil{t.Fatal(err)}
	// Deliberately do not defer Close: os.Exit simulates an abrupt death
	// AFTER a durable terminal, with SQLite recovery left to the next process.
	ledger,err:=inputledger.Open(ctx,domain.ID(run),b,store)
	if err!=nil{t.Fatal(err)}
	refs,err:=policiesByRole(b)
	if err!=nil{t.Fatal(err)}
	coordinator:=New(store,b,admission.Policies{
		AdmissionOrder:refs["admission-order"],
		CausalIdentity:refs["causal-identity"],
		Fence:refs["admission-fence"],Rules:[][]any{},
	})
	done,err:=coordinator.ResumeRegisteredWorkV1(ctx,ledger,run,snapshot)
	if err!=nil{t.Fatal(err)}
	if !done.Continued||len(done.Publication.Canonical)==0||
		done.Cycle.ID!=domain.ID(done.Checkpoint.Action.CycleID){
		t.Fatal("E2 process recovered without exactly one frozen physical terminal")
	}
	os.Exit(e2ExitAfterCommit)
}

func referenceE2CrashAfterTerminal(t *testing.T,path,manifest string,run,snapshot [32]byte){
	t.Helper()
	ctx,cancel:=context.WithTimeout(context.Background(),75*time.Second)
	defer cancel()
	cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestReferenceE2RecoveryProcessChild$","-test.v=false")
	cmd.Env=append(os.Environ(),
		"CSF_E2_CHILD=strict-recovery",
		"CSF_E2_CAUSAL="+path,"CSF_E2_MANIFEST="+manifest,
		"CSF_E2_RUN="+hex.EncodeToString(run[:]),
		"CSF_E2_SNAPSHOT="+hex.EncodeToString(snapshot[:]))
	output,err:=cmd.CombinedOutput()
	if ctx.Err()!=nil{t.Fatalf("E2 child timed out: %v",ctx.Err())}
	var exit *exec.ExitError
	if !errors.As(err,&exit)||exit.ExitCode()!=e2ExitAfterCommit{
		t.Fatalf("E2 recovery child did not exit after committed terminal: %v output=%s",err,output)
	}
	if len(output)>0{t.Fatalf("recovery child unexpectedly emitted privileged data: %q",output)}
}

func referenceE2CausalTranscript(t *testing.T,path string)referenceE1Snapshot{
	t.Helper()
	s,err:=referenceE1Read(context.Background(),[]referenceE1Location{{Name:"causal",Path:path}})
	if err!=nil{t.Fatal(err)}
	return s
}
func referenceE2Count(t *testing.T,path,table string)int{
	t.Helper()
	allow:=map[string]bool{
		"committed_cycles":true,"world_revisions":true,"event_store":true,
		"decision_ledger":true,"source_ingress_receipts":true,
	}
	if !allow[table]{t.Fatal("E2 unknown table")}
	db,err:=openReferenceReadOnlyDB(path)
	if err!=nil{t.Fatal(err)}
	defer db.Close()
	var n int
	if err:=db.QueryRowContext(context.Background(),"SELECT COUNT(*) FROM "+table).Scan(&n);err!=nil{t.Fatal(err)}
	return n
}

func TestReferenceE2SevenPhysicalAtomicFaultsNewProcessRecovery(t *testing.T){
	ctx:=context.Background()
	for step:=uint8(1);step<=7;step++{
		t.Run(fmt.Sprintf("rollback-step-%d",step),func(t *testing.T){
			c,store,ledger,cycle,fence,approved,path,host:=realPhysicalFixture(t)
			_,_,_,_,_,run:=ledger.Snapshot()
			g,err:=store.ValidatedGenesis(ctx,[32]byte(run),host.Bundle)
			if err!=nil{t.Fatal(err)}
			snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),host.Bundle,nil)
			if err!=nil{t.Fatalf("cannot snapshot frozen physical fence: %v",err)}
			pre:=referenceE2CausalTranscript(t,path)
			c.failAfterStep=step
			if _,err:=c.CommitPhysical(ctx,ledger,cycle,fence,approved);err==nil{
				t.Fatal("E2 injected physical commit fault was ignored")
			}
			c.failAfterStep=0
			post:=referenceE2CausalTranscript(t,path)
			if err:=referenceE1Compare(pre,post);err!=nil{
				t.Fatalf("E2 fault leaked partial prefix: %v",err)
			}
			if n:=referenceE2Count(t,path,"committed_cycles");n!=0{
				t.Fatalf("E2 failed attempt published %d CycleCommits",n)
			}
			// An ordinary fresh, uninterrupted reference on the SAME signed
			// A1 input and Genesis, including its in-flight snapshot.
			baselinePath:=filepath.Join(t.TempDir(),"continuous.db")
			continuous,err:=sqlite.Open(ctx,baselinePath)
			if err!=nil{t.Fatal(err)}
			defer continuous.Close()
			if err:=continuous.PersistGenesis(ctx,g,host.Bundle);err!=nil{t.Fatal(err)}
			if err:=continuous.BootstrapClock(ctx,g,host.Bundle);err!=nil{t.Fatal(err)}
			if err:=continuous.BootstrapGenesisTriggers(ctx,g,host.Bundle,nil);err!=nil{t.Fatal(err)}
			baseLedger,err:=inputledger.New(ctx,run,host.Bundle,continuous)
			if err!=nil{t.Fatal(err)}
			baseCoordinator:=New(continuous,host.Bundle,c.admissionPolicies)
			if _,err:=baseCoordinator.StagePhysicalInput(ctx,baseLedger,approved,cycle);err!=nil{t.Fatal(err)}
			baseFence,err:=admission.NewCoordinator(admission.NewFenceLog(continuous)).
				AdmitThenEvaluate(ctx,host.Bundle,baseLedger,cycle,c.admissionPolicies,nil)
			if err!=nil{t.Fatal(err)}
			if _,err:=continuous.CreateSnapshotV1(ctx,[32]byte(run),host.Bundle,nil);err!=nil{t.Fatal(err)}
			if _,err:=baseCoordinator.CommitPhysical(ctx,baseLedger,cycle,baseFence,approved);err!=nil{t.Fatal(err)}
			// No former process memory: only a public manifest on private disk.
			dir:=t.TempDir()
			manifest,err:=host.Bundle.PhysicalManifestCanonical()
			if err!=nil{t.Fatal(err)}
			manifestPath:=filepath.Join(dir,"physical-manifest.json")
			if err:=os.WriteFile(manifestPath,manifest,0600);err!=nil{t.Fatal(err)}
			if err:=store.Close();err!=nil{t.Fatal(err)}
			referenceE2CrashAfterTerminal(t,path,manifestPath,[32]byte(run),[32]byte(snap.ID))
			// New SQLite + InputLedger: double restart must not duplicate.
			reopened,err:=sqlite.Open(ctx,path)
			if err!=nil{t.Fatal(err)}
			defer reopened.Close()
			reloaded,err:=inputledger.Open(ctx,run,host.Bundle,reopened)
			if err!=nil{t.Fatal(err)}
			resumer:=New(reopened,host.Bundle,c.admissionPolicies)
			again,err:=resumer.ResumeRegisteredWorkV1(ctx,reloaded,[32]byte(run),[32]byte(snap.ID))
			if err!=nil||again.Continued{
				t.Fatalf("E2 re-executed physical source after committed process crash: continued=%t err=%v",again.Continued,err)
			}
			if _,err:=reopened.ReplayClock(ctx,[32]byte(run),host.Bundle);err!=nil{t.Fatal(err)}
			if referenceE2Count(t,path,"committed_cycles")!=1||
				referenceE2Count(t,path,"world_revisions")!=2||
				referenceE2Count(t,path,"event_store")!=1{
				t.Fatal("E2 post-crash causal materialization duplicated or lost")
			}
			final:=referenceE2CausalTranscript(t,path)
			direct:=referenceE2CausalTranscript(t,baselinePath)
			if err:=referenceE1Compare(direct,final);err!=nil{
				t.Fatalf("E2 recovered cut differs bytewise from same-source continuous run: %v",err)
			}
		})
	}
}

func TestReferenceE2RecoveredCutCannotCreateNewSourceOrCrossRun(t *testing.T){
	ctx:=context.Background()
	c,store,ledger,cycle,fence,approved,path,host:=realPhysicalFixture(t)
	_,_,_,_,_,run:=ledger.Snapshot()
	_,sources,_,_,_,_:=ledger.Snapshot()
	units,ok:=fence.Fields[11].([]any)
	if !ok||len(units)!=1{t.Fatal("E2 invalid frozen physical fixture")}
	entry,ok:=units[0].([]any)
	if !ok{t.Fatal("E2 invalid frozen source fixture")}
	recovered,err:=c.frozenPhysicalCut(ctx,run,sources,entry)
	if err!=nil||!recovered.IsDurablyRecovered(){t.Fatalf("cannot authenticate durable physical cut: %v",err)}
	before:=referenceE2CausalTranscript(t,path)
	if _,err:=c.StagePhysicalInput(ctx,ledger,recovered,cycle);err==nil{
		t.Fatal("terminal-only recovered cut created new exogenous authority")
	}
	if err:=referenceE1Compare(before,referenceE2CausalTranscript(t,path));err!=nil{
		t.Fatalf("recovered cut stage denial mutated ledger: %v",err)
	}
	g,err:=store.ValidatedGenesis(ctx,[32]byte(run),host.Bundle)
	if err!=nil{t.Fatal(err)}
	cutRaw,signature,completed,err:=approved.CanonicalProof()
	if err!=nil{t.Fatal(err)}
	reg,err:=host.Bundle.PhysicalRegistration()
	if err!=nil{t.Fatal(err)}
	wrong:=run
	wrong[0]^=0x42
	if _,err:=embodimentv1.RecoverApprovedCutFromDurableIngress(cutRaw,signature,completed,reg,wrong,g.Digest);err==nil{
		t.Fatal("E2 foreign RunID authorized a physical replay")
	}
	tampered:=append([]byte(nil),signature...)
	tampered[1]^=0x22
	if _,err:=embodimentv1.RecoverApprovedCutFromDurableIngress(cutRaw,tampered,completed,reg,run,g.Digest);err==nil{
		t.Fatal("E2 forged signature authorized recovery")
	}
	mutant:=append([]any(nil),completed...)
	mutant[3]=make([]byte,32)
	if _,err:=embodimentv1.RecoverApprovedCutFromDurableIngress(cutRaw,signature,mutant,reg,run,g.Digest);err==nil{
		t.Fatal("E2 forged physical body digest authorized recovery")
	}
}

func TestReferenceE2CorruptSavedIngressRefusesRecoveryBeforeCommit(t *testing.T){
	ctx:=context.Background()
	c,store,ledger,_,_,_,path,host:=realPhysicalFixture(t)
	_,_,_,_,_,run:=ledger.Snapshot()
	snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),host.Bundle,nil)
	if err!=nil{t.Fatal(err)}
	if err:=store.Close();err!=nil{t.Fatal(err)}
	// A separate adversarial writer acts only in the TEST. Production
	// recovery uses the SQLite read/verification pathways, never mutations.
	db,err:=sql.Open("sqlite",path)
	if err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"UPDATE input_ledger SET canonical=x'80' WHERE id=(SELECT id FROM input_ledger WHERE canonical<>x'' LIMIT 1)");err!=nil{db.Close();t.Fatal(err)}
	if err:=db.Close();err!=nil{t.Fatal(err)}
	reopened,err:=sqlite.Open(ctx,path)
	if err!=nil{t.Fatal(err)}
	defer reopened.Close()
	reloaded,err:=inputledger.Open(ctx,run,host.Bundle,reopened)
	if err==nil{
		// A loaded ledger is not proof of snapshot correctness.
		recoveredCoordinator := New(reopened, host.Bundle, c.admissionPolicies)
		if _,err:=recoveredCoordinator.ResumeRegisteredWorkV1(ctx,reloaded,[32]byte(run),[32]byte(snap.ID));err==nil{
			t.Fatal("E2 corrupted authoritative source was recovered")
		}
	}
	if n:=referenceE2Count(t,path,"committed_cycles");n!=0{
		t.Fatalf("E2 corrupted ingress produced %d unauthorized commits",n)
	}
}
