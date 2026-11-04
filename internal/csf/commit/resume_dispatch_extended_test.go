package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// Previously the complete mixed cohorts were exercised through direct
// terminals only. These routes prove that persisted fences select the same
// handler after a snapshot, without inventing a second commit path.
func TestCSFRV18RegisteredResumeDispatchesMixedSystemAndResource(t *testing.T){
 ctx:=context.Background()
 t.Run("resource plus trigger and scheduled occurrence",func(t *testing.T){
  c,store,ledger,_,_,_:=singleResourceFixtureWithSystem(t,false,true,true,false)
  _,_,_,_,_,run:=ledger.Snapshot()
  snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
  if err!=nil{t.Fatal(err)}
  result,err:=c.ResumeRegisteredWorkV1(ctx,ledger,[32]byte(run),snapshot.ID)
  if err!=nil||!result.Continued{
   t.Fatalf("mixed resource/system fence was not routed to registered terminal: %+v %v",result,err)
  }
  if _,err:=store.ReplayClock(ctx,[32]byte(run),c.bundle);err!=nil{
   t.Fatal("recovered mixed resource/system commit not replayable:",err)
  }
 })
 t.Run("NoProposal plus scheduled occurrence",func(t *testing.T){
  setup:=func(store *sqlite.Store,run domain.ID,b *policy.Bundle)error{
   genesis:=validGenesis(t,b)
   d:=identity.New(b)
   origin:=[]any{uint64(2),[]any{genesis.ID[:],genesis.Digest[:]}}
   id,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",
    1,[]any{origin,uint64(0),uint64(0)})
   if err!=nil{return err}
   fields:=[]any{id[:],run[:],"schedule",int64(genesis.InitialInstant),
    []any{int64(genesis.InitialInstant),uint64(0)},"test.occurrence",uint64(1),
    genesis.Payload[16],uint64(0),origin,uint64(0),uint64(0),
    []any{uint64(0)},[]any{uint64(0)}}
   digest,_,err:=d.Sum("cote.csf.digest.occurrence-unit",
    "cote.csf.schema.scheduled-occurrence.body",1,fields)
   if err!=nil{return err}
   bytes,err:=codec.CanonicalPayload(append(cloneAny(fields),digest[:]))
   if err!=nil{return err}
   return store.SeedGenesisOccurrence(ctx,domain.ID(id),run,
    domain.EligibilityCoordinate{NotBeforeInstant:genesis.InitialInstant},bytes)
  }
  c,store,ledger,_,fence,_:=noProposalWorkFixtureWithSetup(t,setup)
  if len(fence.Fields[11].([]any))!=3{
   t.Fatal("NoProposal/system admission not a complete three-source cohort")
  }
  _,_,_,_,_,run:=ledger.Snapshot()
  snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
  if err!=nil{t.Fatal(err)}
  completed,err:=c.ResumeRegisteredWorkV1(ctx,ledger,[32]byte(run),snap.ID)
  if err!=nil||!completed.Continued{
   t.Fatalf("NoProposal/system work was not recovered as one atomic terminal: %+v %v",completed,err)
  }
  if _,err:=store.ReplayClock(ctx,[32]byte(run),c.bundle);err!=nil{
   t.Fatal("recovered NoProposal/system terminal not replayable:",err)
  }
  again,err:=c.ResumeRegisteredWorkV1(ctx,ledger,[32]byte(run),snap.ID)
  if err!=nil||again.Continued{
   t.Fatalf("repeat resume emitted a second mixed terminal: %+v %v",again,err)
  }
 })
}
