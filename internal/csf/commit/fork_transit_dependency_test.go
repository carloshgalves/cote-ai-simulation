package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 _ "modernc.org/sqlite"
)

func TestCSFRV18InTransitMigrationPreflightAuthenticatesFullParentChain(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 graph,err:=store.PrepareInTransitDependenciesV1(ctx,[32]byte(run),snapshot.ID,c.bundle)
 if err!=nil{t.Fatal("verified in-transit graph:",err)}
 if len(graph)!=1{t.Fatalf("expected exactly one in-transit dependency, got %d",len(graph))}
 got:=graph[0]
 if got.ParentRun!=[32]byte(run)||got.TransmissionID==got.ClaimID||
  got.OccurrenceID==got.TransmissionID||got.SentCommitID==([32]byte{}){
  t.Fatal("transit dependency identities not isolated")
 }
 for _,item:=range []struct{label string;id [32]byte;raw []byte}{
  {"transmission",got.TransmissionID,got.Transmission},
  {"claim",got.ClaimID,got.Claim},
  {"occurrence",got.OccurrenceID,got.Occurrence},
 }{
  value,err:=codec.StrictDecodePayload(item.raw)
  if err!=nil{t.Fatalf("%s: %v",item.label,err)}
  fields,ok:=value.([]any)
  if !ok||!bytes.Equal(fields[0].([]byte),item.id[:]){
   t.Fatalf("%s graph is not ID-bound",item.label)
  }
 }
 // The extraction is a read-only value graph, not a mutable parent or
 // future child authority. Tampering the returned bytes must not modify the
 // authoritative source or influence a second verified extraction.
 graph[0].Claim[0]^=0x7f
 again,err:=store.PrepareInTransitDependenciesV1(ctx,[32]byte(run),snapshot.ID,c.bundle)
 if err!=nil||len(again)!=1||bytes.Equal(graph[0].Claim,again[0].Claim){
  t.Fatal("migration preflight aliasing or compromised source:",err)
 }
 db,err:=sql.Open("sqlite",path)
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if _,err:=db.ExecContext(ctx,"UPDATE communication_transmissions SET claim_id=zeroblob(32) WHERE status=0");err!=nil{t.Fatal(err)}
 if _,err:=store.PrepareInTransitDependenciesV1(ctx,[32]byte(run),snapshot.ID,c.bundle);err==nil{
  t.Fatal("in-transit migration preflight accepted a forged Claim authority")
 }
}

func TestCSFRV18TransitPreparationRejectsLaterSameRevisionInputTail(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,_:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 if graph,err:=store.PrepareInTransitDependenciesV1(ctx,[32]byte(run),snap.ID,c.bundle);err!=nil||len(graph)!=1{
  t.Fatalf("baseline exact-cut preflight failed: %v",err)
 }
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 genesis:=stockGenesis(t,c.bundle)
 birth:=[]any{uint64(2),[]any{genesis.ID[:],genesis.Digest[:]}}
 _,err=ledger.DeclareRound(ctx,"schedule",[2]string{"school","b"},[]string{"later"},
  domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+240},
  birth,5,90,0,[]any{uint64(0)})
 if err!=nil{t.Fatal("legitimate post-snapshot pending round:",err)}
 current,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||current!=state{t.Fatal("new input unexpectedly advanced the world revision",err)}
 if _,err:=store.PrepareInTransitDependenciesV1(ctx,[32]byte(run),snap.ID,c.bundle);err==nil{
  t.Fatal("migration combined an old checkpoint roster with newer same-revision input ledger")
 }
}
