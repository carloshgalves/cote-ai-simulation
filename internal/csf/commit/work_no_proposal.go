package commit

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// CommitNoProposal is the first successful WORK terminal slice. Its
// narrowness is deliberate: every admitted unit must be a real, recorded
// NoProposal, so there is no world mutation, EventDraft, or PerceptionTask
// to invent. A subsequent extension must use the SAME CommitWork authority
// when it adds reducer-owned events and lifecycle transitions.
func (c *Coordinator) CommitNoProposal(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var empty ports.CanonicalRecord
 if c==nil||c.bundle==nil||c.store==nil||ledger==nil||!ledger.SharesFenceAuthority(c.store){
  return empty,errors.New("NoProposal work requires shared authoritative store")
 }
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return empty,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return empty,err}
 persisted,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(persisted,fence.Canonical){return empty,errors.New("NoProposal fence is not durably published")}
 state,err:=c.store.LoadClock(ctx,[32]byte(cycle.ID)) // never substitute cycle ID for run ID
 _=state
 _=err
 _,_,_,rounds,slots,runID:=view.Snapshot()
 if runID==cycle.ID{return empty,errors.New("cycle identity must differ from run identity")}
 state,err=c.store.LoadClock(ctx,[32]byte(runID));if err!=nil{return empty,err}
 if state.Instant!=cycle.Instant||state.Revision!=cycle.BaseRevision||state.LastOrdinal>=int64(cycle.Ordinal)||
  state.Revision==domain.WorldRevision(math.MaxUint64)||cycle.Ordinal>math.MaxInt64{
  return empty,errors.New("stale or overflowing WORK clock")
 }
 genesisBytes,err:=c.store.LoadGenesis(ctx,[32]byte(runID));if err!=nil{return empty,err}
 g,err:=genesis.ValidateCanonical(genesisBytes,c.bundle);if err!=nil{return empty,err}
 typed,ok:=g.Payload[16].([]any);if !ok||len(typed)!=4{return empty,errors.New("invalid genesis world state")}
 hash,err:=bytes32(typed[3]);if err!=nil{return empty,err}
 if !bytes.Equal(cycle.BaseStateHash[:],hash[:]){return empty,errors.New("NoProposal requires unchanged typed world state")}
 byID:=map[domain.ID]inputledger.Response{}
 for _,round:=range rounds{
  for _,response:=range slots[round.ID]{
   if _,dup:=byID[response.ID];dup{return empty,errors.New("duplicate recorded slot root")}
   byID[response.ID]=response
  }
 }
 unitValues,ok:=fence.Fields[11].([]any);if !ok{return empty,errors.New("fence admitted source list missing")}
 d:=identity.New(c.bundle)
 policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
 decisions:=make([]ports.CanonicalRecord,0,len(unitValues))
 ids,pairs,admitted:=make([]any,0,len(unitValues)),make([]any,0,len(unitValues)),make([]any,0,len(unitValues))
 var prev domain.ID
 for i,raw:=range unitValues{
  entry,ok:=raw.([]any)
  if !ok||len(entry)!=5||entry[1]!=uint64(0){return empty,errors.New("WORK unit is not an admitted slot response")}
  b,ok:=entry[3].([]byte);if !ok||len(b)!=32{return empty,errors.New("invalid source response ID")}
  var id domain.ID;copy(id[:],b)
  if i>0&&id==prev{return empty,errors.New("duplicate source response")}
  prev=id
  response,found:=byID[id]
  if !found||response.Kind!=1||!bytes.Equal(entry[4].([]byte),response.Digest[:]){
   return empty,errors.New("cannot commit proposal or an unauthenticated NoProposal as zero-effect WORK")
  }
  decisionID,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
   []any{cycle.ID[:],uint64(cycle.Attempt),id[:]})
  if err!=nil{return empty,err}
  recordFields:=[]any{decisionID[:],runID[:],cycle.ID[:],uint64(cycle.Attempt),
   []any{uint64(0),uint64(1),id[:],response.Digest[:]},uint64(3),
   []any{uint64(0)},"slot.no-proposal",[]any{},policies["validators"],
   policies["conflict-resolvers"],[]any{},[]any{},[]any{uint64(0)},[]any{uint64(0)}}
  recordHash,_,err:=d.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,recordFields)
  if err!=nil{return empty,err}
  canonical,err:=codec.CanonicalPayload(append(cloneAny(recordFields),recordHash[:]))
  if err!=nil{return empty,err}
  decisions=append(decisions,ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:canonical})
  ids=append(ids,decisionID[:])
  pairs=append(pairs,[]any{id[:],decisionID[:],recordHash[:]})
  admitted=append(admitted,id[:])
 }
 if len(unitValues)==0{return empty,errors.New("NoProposal WORK needs admitted slot responses")}
 decisionDigest,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,pairs)
 if err!=nil{return empty,err}
 taskDigest,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,[]any{})
 if err!=nil{return empty,err}
 next:=state
 next.Revision++
 next.LastOrdinal=int64(cycle.Ordinal)
 originalClock,err:=clock.Encode(state);if err!=nil{return empty,err}
 nextClock,err:=clock.Encode(next);if err!=nil{return empty,err}
 revision,err:=codec.CanonicalPayload([]any{uint64(state.Revision),typed,hash[:],
  uint64(next.Revision),typed,hash[:]});if err!=nil{return empty,err}
 journalBody:=[]any{runID[:],cycle.ID[:],uint64(cycle.Attempt),int64(cycle.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(cycle.BaseRevision),hash[:],
  uint64(next.Revision),hash[:],fence.Digest[:],admitted,fence.InputDigest[:],
  policies["cycle-coordinate"],policies["event-order"],[]any{},ids,decisionDigest[:],
  []any{},taskDigest[:],policies["perception"],policies["perception-identity"],
  uint64(next.NextLogicalSequence)}
 batchHash,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{journalBody,[]any{}});if err!=nil{return empty,err}
 commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{runID[:],cycle.ID[:],uint64(cycle.Attempt)});if err!=nil{return empty,err}
 journalBytes,err:=codec.CanonicalPayload(append(cloneAny(journalBody),batchHash[:]))
 if err!=nil{return empty,err}
 journal:=ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalBytes}
 batch:=ports.WorkBatch{RunID:[32]byte(runID),CycleID:[32]byte(cycle.ID),
  FenceID:[32]byte(fence.ID),ExpectedClock:originalClock,ResultClock:nextClock,
  ExpectedInputPrefix:prefix,Journal:journal,Decisions:decisions,
  BaseStateHash:hash,RevisionCanonical:revision,FailAfterStep:c.failAfterStep}
 if err:=c.store.CommitWork(ctx,batch);err!=nil{return empty,fmt.Errorf("NoProposal WORK transaction rejected: %w",err)}
 return journal,nil
}
