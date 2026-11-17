//go:build linux

package commit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

// The producer is called only twice: those two genuine A1 signed cuts are
// held constant between the uninterrupted reference and every interrupted
// continuation. This avoids treating independent A1 wall_time as equivalent.
func referenceE2SignedPair(t *testing.T) (physicalhost.ProducerHost, [2]physicalhost.ProducerPlan, [2]embodimentv1.ApprovedCut) {
	t.Helper()
	host, plans := referenceD2TestHost(t)
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil { t.Fatal(err) }
	host.GenesisCanonical = g.CanonicalBytes
	var cuts [2]embodimentv1.ApprovedCut
	for i := range cuts {
		source := host
		source.Epoch = uint64(i+1)
		source.Plan = plans[i]
		cuts[i],err = physicalhost.RunSupervisedPhysical(context.Background(),source)
		if err != nil { t.Fatalf("real supervised A1 cut %d: %v",i+1,err) }
		if _,_,_,err := cuts[i].CanonicalProof(); err != nil { t.Fatal(err) }
	}
	host.GenesisCanonical = nil
	return host,plans,cuts
}

func referenceE2Paths(t *testing.T)(string,string){
	t.Helper()
	dir:=t.TempDir()
	owners:=filepath.Join(dir,"private-owners")
	if err:=os.Mkdir(owners,0700);err!=nil{t.Fatal(err)}
	return filepath.Join(dir,"run.db"),filepath.Join(owners,"actor-b.db")
}

func referenceE2Capture(t *testing.T,db,owner string) referenceE1Snapshot {
	t.Helper()
	s,err:=referenceE1Read(context.Background(),[]referenceE1Location{
		{Name:"causal",Path:db},
		{Name:"owner-a",Path:owner+".actor-a"},
		{Name:"owner-b",Path:owner},
	})
	if err!=nil{t.Fatal(err)}
	if err:=s.requireCausalAndOwners();err!=nil{t.Fatal(err)}
	return s
}

func TestReferenceE2FourteenAtomicPhysicalFaultsReopenAndConvergeOnExactD3(t *testing.T){
	ctx:=context.Background()
	host,plans,cuts:=referenceE2SignedPair(t)
	basePath,baseOwner:=referenceE2Paths(t)
	baseline,err:=RunReferenceD3(ctx,basePath,baseOwner,bundlePath,
		ReferenceD2Config{Host:host,EpochPlans:plans,frozenSupervisedCuts:&cuts},ReferenceOptions{})
	if err!=nil{t.Fatal(err)}
	original:=referenceE2Capture(t,basePath,baseOwner)
	if _,err:=baseline.PublicReport();err!=nil{t.Fatal(err)}
	for epoch:=1;epoch<=2;epoch++{
		for step:=uint8(1);step<=7;step++{
			t.Run(fmt.Sprintf("epoch%d-commitwork-step%d",epoch,step),func(t *testing.T){
				db,owner:=referenceE2Paths(t)
				_,err:=RunReferenceD2(ctx,db,owner,bundlePath,ReferenceD2Config{
					Host:host,EpochPlans:plans,frozenSupervisedCuts:&cuts,
					FailPhysicalAtEpoch:epoch,FailPhysicalAfterStep:step,
				},ReferenceOptions{})
				if err==nil||!strings.Contains(err.Error(),"CommitPhysical"){
					t.Fatalf("injected physical failure did not stop terminal: %v",err)
				}
				// The first runner and its SQLite connections are gone.
				// E2 must reconstruct cycle/fence from persisted authorities,
				// reusing the authentic cut but NEVER rerunning Python.
				if err:=referenceE2RecoverPhysical(ctx,db,host.Bundle,cuts,epoch);err!=nil{
					t.Fatalf("durable E2 restart failed: %v",err)
				}
				after:=referenceE2Capture(t,db,owner)
				if err:=referenceE1Compare(original,after);err!=nil{
					t.Fatalf("E2 recovery diverged from uninterrupted same-source D3: %v",err)
				}
				// D3 audits the recovered *physical + cognitive* run, not
				// merely a synthetic ninth revision or a stock-only fixture.
				audited,err:=AuditReferenceD3(ctx,db,host.Bundle,baseline.d2)
				if err!=nil{t.Fatalf("recovered D3 privacy/replay audit: %v",err)}
				if _,err:=audited.PublicReport();err!=nil{t.Fatal(err)}
			})
		}
	}
}

// After a signed cut has been persisted and fenced, a wrong epoch cannot
// settle the frozen causal cut. This denial must leave the entire E1
// multiset of records untouched, even across an independent SQLite reopen.
func TestReferenceE2TamperedRestartAttemptDoesNotChangePrefix(t *testing.T) {
	ctx:=context.Background()
	host,plans,cuts:=referenceE2SignedPair(t)
	db,owner:=referenceE2Paths(t)
	_,err:=RunReferenceD2(ctx,db,owner,bundlePath,ReferenceD2Config{
		Host:host,EpochPlans:plans,frozenSupervisedCuts:&cuts,
		FailPhysicalAtEpoch:1,FailPhysicalAfterStep:4},ReferenceOptions{})
	if err==nil{t.Fatal("missing injected abort")}
	pre,err:=referenceE1Read(ctx,[]referenceE1Location{{Name:"causal",Path:db}})
	if err!=nil{t.Fatal(err)}
	if err:=referenceE2RecoverWrongEpoch(ctx,db,host.Bundle,cuts[1]);err==nil{
		t.Fatal("future physical epoch accepted as current recovery source")
	}
	post,err:=referenceE1Read(ctx,[]referenceE1Location{{Name:"causal",Path:db}})
	if err!=nil{t.Fatal(err)}
	if err:=referenceE1Compare(pre,post);err!=nil{
		t.Fatalf("denied wrong epoch altered authoritative persisted prefix: %v",err)
	}
	if err:=referenceE2RecoverPhysical(ctx,db,host.Bundle,cuts,1);err!=nil{t.Fatal(err)}
	audited,err:=referenceE1Read(ctx,[]referenceE1Location{{Name:"causal",Path:db}})
	if err!=nil{t.Fatal(err)}
	if len(audited.Databases["causal"].Tables["committed_cycles"].Rows)!=9{
		t.Fatal("recovery did not produce exactly nine complete atomic commits")
	}
	_ = owner
}

func TestReferenceE2NoReexecutionOfPhysicalAfterRestart(t *testing.T){
	ctx:=context.Background()
	host,plans,cuts:=referenceE2SignedPair(t)
	db,owner:=referenceE2Paths(t)
	_,err:=RunReferenceD3(ctx,db,owner,bundlePath,
		ReferenceD2Config{Host:host,EpochPlans:plans,frozenSupervisedCuts:&cuts},ReferenceOptions{})
	if err!=nil{t.Fatal(err)}
	before:=referenceE2Capture(t,db,owner)
	if err:=referenceE2RecoverPhysical(ctx,db,host.Bundle,cuts,2);!errors.Is(err,errReferenceE2AlreadyCommitted){
		t.Fatalf("restarted terminal was allowed to double-settle ninth commit: %v",err)
	}
	after:=referenceE2Capture(t,db,owner)
	if err:=referenceE1Compare(before,after);err!=nil{t.Fatalf("duplicate recovery mutated SQLite: %v",err)}
	reopened,err:=sqlite.Open(ctx,db)
	if err!=nil{t.Fatal(err)}
	defer reopened.Close()
	g,err:=buildReferenceGenesis(host.Bundle)
	if err!=nil{t.Fatal(err)}
	replay,err:=reopened.ReplayClock(ctx,[32]byte(g.RunID),host.Bundle)
	if err!=nil||replay.ReplayedCommits!=9{t.Fatalf("post-restart replay changed world: %+v %v",replay,err)}
}
