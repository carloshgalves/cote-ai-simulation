package commit

import (
 "bytes"
 "context"
 "encoding/hex"
 "encoding/json"
 "os"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
)

const bundlePath="../../../docs/architecture/canonical-codec-v1-bundle"

func validGenesis(t *testing.T,b *policy.Bundle)*genesis.Genesis{
 t.Helper()
 data,err:=os.ReadFile(bundlePath+"/fixtures.json")
 if err!=nil{t.Fatal(err)}
 var fixtures struct{Semantic []struct{CaseID string `json:"case_id"`;GenesisEnvelopeHex string `json:"genesis_envelope_hex"`} `json:"semantic"`}
 if err:=json.Unmarshal(data,&fixtures);err!=nil{t.Fatal(err)}
 for _,item:=range fixtures.Semantic{
  if item.CaseID!="genesis.pinning"{continue}
  encoded,err:=hex.DecodeString(item.GenesisEnvelopeHex);if err!=nil{t.Fatal(err)}
  env,err:=codec.StrictDecodeEnvelope(encoded);if err!=nil{t.Fatal(err)}
  p:=env.Value.([]any)
  p[15]=[]any{[]any{"cote.csf.schema.snapshot-pending-state",uint64(1)}}
  state:=make([]any,10);for i:=range state{state[i]=[]any{}}
  stateEnv,err:=codec.CanonicalBytes("cote.csf.snapshot.pending-state","cote.csf.schema.snapshot-pending-state",1,state)
  if err!=nil{t.Fatal(err)}
  digest:=identity.OpaqueContentHash(stateEnv)
  p[16]=[]any{"cote.csf.schema.snapshot-pending-state",uint64(1),stateEnv,digest[:]}
  encoded,err=codec.CanonicalBytes("cote.csf.digest.genesis-manifest",env.SchemaID,env.SchemaVersion,p)
  if err!=nil{t.Fatal(err)}
  g,err:=genesis.ValidateCanonical(encoded,b)
  if err!=nil{t.Fatal(err)}
  return g
 }
 t.Fatal("canonical genesis fixture missing")
 return nil
}

func testFixture(t *testing.T)(*Coordinator,*sqlite.Store,*inputledger.Ledger,domain.ID,*policy.Bundle) {
 t.Helper()
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 store,err:=sqlite.Open(ctx,t.TempDir()+"/clock.db");if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 d:=identity.New(bundle)
 due:=g.InitialInstant+100
 origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 occID,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,[]any{origin,uint64(0),uint64(0)})
 if err!=nil{t.Fatal(err)}
 fields:=[]any{occID[:],g.RunID[:],"schedule",int64(due),[]any{int64(due),uint64(1)},"test.occurrence",uint64(1),
 g.Payload[16],uint64(0),origin,uint64(0),uint64(0),[]any{uint64(0)},[]any{uint64(0)}}
 dig,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields)
 if err!=nil{t.Fatal(err)}
 raw,err:=codec.CanonicalPayload(append(fields,dig[:]));if err!=nil{t.Fatal(err)}
 if err:=store.SeedGenesisOccurrence(ctx,domain.ID(occID),g.RunID,domain.EligibilityCoordinate{NotBeforeInstant:due,NotBeforeCycleOrdinal:1},raw);err!=nil{t.Fatal(err)}
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"driver"});err!=nil{t.Fatal(err)}
 closedAt:=domain.EligibilityCoordinate{NotBeforeInstant:due,NotBeforeCycleOrdinal:1}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"closed-through-next",ClosedThrough:closedAt});err!=nil{t.Fatal(err)}
 roles,err:=policiesByRole(bundle);if err!=nil{t.Fatal(err)}
 policies:=admission.Policies{AdmissionOrder:roles["admission-order"],CausalIdentity:roles["causal-identity"],Fence:roles["admission-fence"],Rules:[][]any{}}
 return New(store,bundle,policies),store,ledger,g.RunID,bundle
}

func TestClockAdvancePublishesCycleAndPendingActivation(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,runID,_:=testFixture(t)
 before,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 var triggerID domain.ID;triggerID[0]=0x80
 definition:=trigger.Definition{ID:triggerID,Version:1,Policy:trigger.RisingEdge,FireOnInitialTrue:true,Dependencies:[]string{"clock"}}
 prior:=trigger.Runtime{DefinitionID:triggerID,Version:1,Armed:true}
 result,err:=coordinator.Advance(ctx,ledger,runID,[]TriggerWork{{Definition:definition,Previous:prior,Predicate:func(clock.State)bool{return true}}})
 if err!=nil{t.Fatal(err)}
 if result.Plan.Kind!=clock.ClockAdvance||result.ResultClock.Instant<=before.Instant||result.ResultClock.LastOrdinal!=-1{t.Fatal(result.Plan,result.ResultClock)}
 if len(result.Events)!=1||len(result.Activations)!=1{t.Fatalf("events=%d activations=%d",len(result.Events),len(result.Activations))}
 if result.Commit.Identity==([32]byte{}){t.Fatal("zero commit id")}
 again,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 if again!=result.ResultClock||again.Revision!=1||again.NextLogicalSequence!=1{t.Fatal(again)}
 pending,err:=store.PendingClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 next,ok,err:=clock.Next(again,pending)
 if err!=nil||!ok||next.Kind!=clock.Work||next.OriginOrdinal!=0 {t.Fatal(next,err)}
 if _,err:=coordinator.Advance(ctx,ledger,runID,nil);err==nil {t.Fatal("advance repeated while WORK activation is pending")}
 fence,err:=store.GetFence(ctx,result.Fence.ID)
 if err!=nil||!bytes.Equal(fence,result.Fence.Canonical){t.Fatal("fence changed")}
}
