// Package commit provides the only CLOCK_ADVANCE builder and transaction caller.
// The WORK/settlement branch will be added by CSFRV1-4/5; it is NOT a second
// publication authority.
package commit

import (
    "context"
    "encoding/hex"
    "errors"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

// Store must be the SAME adapter instance passed to InputLedger and FenceLog.
// No cross-database transaction is supported or assumed in CSFRV1-3.
type Store interface {
    ports.AppendOnlyStore
    ports.RecordListingStore
    ports.AdmissionFenceStore
    ports.PrefixConditionalFenceStore
    ports.ClockAdvanceStore
    LoadClock(context.Context,[32]byte)(clock.State,error)
    PendingClock(context.Context,[32]byte)([]clock.Pending,error)
    LoadGenesis(context.Context,[32]byte)([]byte,error)
}

type TriggerWork struct {
    Definition trigger.Definition
    Previous trigger.Runtime
    // Pure, versioned predicates may read ONLY post-state and declared
    // dependencies. They must not consult wall clock, LLM or mutable stores.
    Predicate func(clock.State) bool
    PreviousCanonical []byte
}

type Outcome struct {
    Plan clock.Plan
    Fence admission.Fence
    Commit ports.CanonicalRecord
    Events []ports.CanonicalRecord
    Activations []ports.CanonicalRecord
    ResultClock clock.State
}

type Coordinator struct {
    store Store
    bundle *policy.Bundle
    admissionPolicies admission.Policies
}

func New(store Store,bundle *policy.Bundle,policies admission.Policies)*Coordinator {
    return &Coordinator{store:store,bundle:bundle,admissionPolicies:policies}
}

func policiesByRole(bundle *policy.Bundle)(map[string][]any,error){
    roles,err:=bundle.RequiredPolicyRoles();if err!=nil{return nil,err}
    refs,err:=bundle.GenesisPolicyRefs();if err!=nil{return nil,err}
    if len(roles)!=len(refs){return nil,errors.New("policy role and ref count differ")}
    result:=map[string][]any{}
    for i,r:=range roles{result[r]=refs[i]}
    return result,nil
}

func eventValue(from,to domain.SimulationInstant)([]any,error){
    // Versioned, synthetic time-advance payload; no physical telemetry or
    // perceptual task. Its schema is local to the CLOCK_ADVANCE slice.
    data,err:=codec.CanonicalBytes("cote.csf.event.clock-advanced","cote.csf.schema.clock-advanced",1,[]any{int64(from),int64(to)})
    if err!=nil{return nil,err}
    h:=identity.OpaqueContentHash(data)
    return []any{"cote.csf.schema.clock-advanced",uint64(1),data,h[:]},nil
}
func bytes32(v any)([32]byte,error){
    var dst [32]byte
    b,ok:=v.([]byte);if !ok||len(b)!=32{return dst,errors.New("expected a pinned 32-byte digest")}
    copy(dst[:],b);return dst,nil
}
func cloneAny(x []any)[]any {v:=make([]any,len(x));copy(v,x);return v}

func (c *Coordinator) Advance(ctx context.Context,ledger *inputledger.Ledger,runID domain.ID,triggerWork []TriggerWork)(Outcome,error){
    var result Outcome
    if c==nil||c.store==nil||c.bundle==nil||ledger==nil{return result,errors.New("clock coordinator requires store, bundle and input ledger")}
    if !ledger.SharesFenceAuthority(c.store){return result,errors.New("input ledger and clock coordinator must share adapter instance")}
    state,err:=c.store.LoadClock(ctx,[32]byte(runID));if err!=nil{return result,err}
    pending,err:=c.store.PendingClock(ctx,[32]byte(runID));if err!=nil{return result,err}
    plan,has,err:=clock.Next(state,pending);if err!=nil{return result,err}
    if !has||plan.Kind!=clock.ClockAdvance{return result,errors.New("no CLOCK_ADVANCE due; WORK or IDLE belongs to later ticket")}
    deriver:=identity.New(c.bundle)
    cycleID,_,err:=deriver.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,[]any{runID[:],[]any{int64(plan.From),uint64(plan.OriginOrdinal)}})
    if err!=nil{return result,err}
    clockPlan,err:=clock.EncodePlan(plan);if err!=nil{return result,err}
    initial,err:=c.store.LoadGenesis(ctx,[32]byte(runID));if err!=nil{return result,err}
    g,err:=genesis.ValidateCanonical(initial,c.bundle);if err!=nil{return result,err}
    if g.RunID!=runID{return result,errors.New("clock run ID differs from persisted genesis")}
    typed,ok:=g.Payload[16].([]any)
    if !ok||len(typed)!=4{return result,errors.New("genesis initial state not a typed value")}
    baseHash,err:=bytes32(typed[3]);if err!=nil{return result,err}
    // This slice publishes only temporal change: synthetic body.advanced is
    // NOT a physical reducer. World-state bytes/hash stay unchanged.
    cycle:=admission.Cycle{
        ID:domain.ID(cycleID),Attempt:0,Instant:state.Instant,Ordinal:plan.OriginOrdinal,
        Plan:clockPlan,BaseRevision:state.Revision,BaseStateHash:domain.Digest(baseHash),
        RetryRef:[]any{uint64(0)},
    }
    fenceCoord:=admission.NewCoordinator(admission.NewFenceLog(c.store))
    var applied ports.ClockAdvanceBatch
    fence,err:=fenceCoord.AdmitThenEvaluate(ctx,c.bundle,ledger,cycle,c.admissionPolicies,func(f admission.Fence)error{
        prepared,err:=c.buildBatch(ctx,runID,state,plan,f,typed,triggerWork)
        if err!=nil{return err}
        applied=prepared
        return c.store.CommitClockAdvance(ctx,prepared)
    })
    if err!=nil{return Outcome{},err}
    after,err:=clock.Decode(applied.ResultClock);if err!=nil{return Outcome{},err}
    return Outcome{Plan:plan,Fence:fence,Commit:applied.Journal,Events:applied.Events,Activations:applied.Activations,ResultClock:after},nil
}

func (c *Coordinator) buildBatch(ctx context.Context,runID domain.ID,state clock.State,plan clock.Plan,f admission.Fence,baseTyped []any,work []TriggerWork)(ports.ClockAdvanceBatch,error){
    var empty ports.ClockAdvanceBatch
    d:=identity.New(c.bundle)
    next,err:=clock.ApplyAdvance(state,plan);if err!=nil{return empty,err}
    beforeBytes,err:=clock.Encode(state);if err!=nil{return empty,err}
    afterBytes,err:=clock.Encode(next);if err!=nil{return empty,err}
    origin,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,[]any{runID[:],[]any{int64(plan.From),uint64(plan.OriginOrdinal)}})
    if err!=nil{return empty,err}
    eventOrder:=[]any{uint64(0),origin[:],"clock",uint64(1),uint64(0),uint64(0)}
    eventID,_,err:=d.Sum("cote.csf.id.event","cote.csf.schema.event-id-preimage",1,[]any{runID[:],origin[:],eventOrder})
    if err!=nil{return empty,err}
    payload,err:=eventValue(plan.From,plan.To);if err!=nil{return empty,err}
    eventBody:=[]any{
        eventID[:],runID[:],eventOrder,"clock.advanced",uint64(1),int64(plan.From),
        uint64(state.NextLogicalSequence),origin[:],uint64(state.Revision),payload,
        []any{},[]any{},[]any{},[]any{},[]any{uint64(0)},uint64(2),"clock",uint64(1),
    }
    eventDigest,_,err:=d.Sum("cote.csf.digest.event","cote.csf.schema.event.body",1,eventBody)
    if err!=nil{return empty,err}
    eventBytes,err:=codec.CanonicalPayload(append(cloneAny(eventBody),eventDigest[:]));if err!=nil{return empty,err}
    event:=ports.CanonicalRecord{Identity:[32]byte(eventID),Canonical:eventBytes}
    noDecisions:=[]any{}
    decisionDigest,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,noDecisions);if err!=nil{return empty,err}
    emptyTasks:=[]any{}
    taskDigest,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,emptyTasks);if err!=nil{return empty,err}
    policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
    fb:=f.InputDigest
    planBytes,err:=clock.EncodePlan(plan);if err!=nil{return empty,err}
    commitBody:=[]any{
        runID[:],origin[:],uint64(0),int64(state.Instant),uint64(plan.OriginOrdinal),planBytes,
        uint64(state.Revision),f.Fields[8],uint64(next.Revision),f.Fields[8],
        f.Digest[:],[]any{},fb[:],policies["cycle-coordinate"],policies["event-order"],
        []any{eventID[:]},[]any{},decisionDigest[:],[]any{},taskDigest[:],
        policies["perception"],policies["perception-identity"],uint64(next.NextLogicalSequence),
    }
    batchDigest,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,[]any{commitBody,[]any{eventBytes}})
    if err!=nil{return empty,err}
    journalBytes,err:=codec.CanonicalPayload(append(cloneAny(commitBody),batchDigest[:]));if err!=nil{return empty,err}
    commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,[]any{runID[:],origin[:],uint64(0)});if err!=nil{return empty,err}
    canonicalState,ok:=baseTyped[2].([]byte);if !ok{return empty,errors.New("genesis state envelope malformed")}
    stateDigest,err:=bytes32(baseTyped[3]);if err!=nil{return empty,err}
    revisionCanonical,err:=codec.CanonicalPayload([]any{uint64(state.Revision),baseTyped,stateDigest[:],uint64(next.Revision),baseTyped,stateDigest[:]})
    if err!=nil{return empty,err}
    inputs,err:=c.store.Records(ctx);if err!=nil{return empty,err}
    _=canonicalState
    changes:=make([]ports.TriggerStateChange,0,len(work))
    activations:=make([]ports.CanonicalRecord,0)
    for _,item:=range work {
        if item.Predicate==nil{return empty,errors.New("nil trigger predicate")}
        fired,err:=trigger.Evaluate(item.Definition,item.Previous,next.Instant,next.Revision,item.Predicate(next),[]trigger.Event{{ID:domain.ID(eventID),OrderKey:eventBytes,Footprint:[]string{"clock"}}})
        if err!=nil{return empty,err}
        runtime:=fired.Runtime
        prior:=item.PreviousCanonical
        after,err:=encodeRuntime(runtime);if err!=nil{return empty,err}
        changes=append(changes,ports.TriggerStateChange{DefinitionID:[32]byte(item.Definition.ID),Before:prior,After:after})
        if !fired.Fired{continue}
        activationID,_,err:=d.Sum("cote.csf.id.trigger-activation","cote.csf.schema.trigger-activation-id-preimage",1,[]any{runID[:],item.Definition.ID[:],uint64(item.Definition.Version),uint64(next.Revision),runtime.ActivationCount})
        if err!=nil{return empty,err}
        pbytes,err:=codec.CanonicalBytes("cote.csf.trigger.predicate","cote.csf.schema.typed-value",1,item.Predicate(next))
        if err!=nil{return empty,err}
        predicateContentDigest:=identity.OpaqueContentHash(pbytes)
        value:=[]any{"cote.csf.schema.typed-value",uint64(1),pbytes,predicateContentDigest[:]}
        predicateDigest,_,err:=d.Sum("cote.csf.digest.trigger-predicate-result","cote.csf.schema.typed-value",1,value)
        if err!=nil{return empty,err}
        originRef:=[]any{uint64(0),origin[:],uint64(next.Revision),anyIDs(fired.CausingEventIDs)}
        source:="trigger:"+hex.EncodeToString(item.Definition.ID[:])
        fields:=[]any{activationID[:],runID[:],source,item.Definition.ID[:],uint64(item.Definition.Version),originRef,
            []any{int64(next.Instant),uint64(0)},runtime.ActivationCount,predicateDigest[:],uint64(0),[]any{uint64(0)}}
        digest,_,err:=d.Sum("cote.csf.digest.trigger-activation-unit","cote.csf.schema.trigger-activation.body",1,fields)
        if err!=nil{return empty,err}
        raw,err:=codec.CanonicalPayload(append(cloneAny(fields),digest[:]));if err!=nil{return empty,err}
        activations=append(activations,ports.CanonicalRecord{Identity:[32]byte(activationID),Canonical:raw})
    }
    return ports.ClockAdvanceBatch{
        RunID:[32]byte(runID),CycleID:[32]byte(origin),FenceID:f.ID,
        TargetInstant:int64(plan.To),TargetOrdinal:uint64(plan.Target.NotBeforeCycleOrdinal),
        ExpectedInputPrefix:inputs,ExpectedRevision:uint64(state.Revision),ResultRevision:uint64(next.Revision),
        ExpectedClock:beforeBytes,ResultClock:afterBytes,
        BaseStateHash:stateDigest,ResultStateHash:stateDigest,
        Journal:ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalBytes},
        Events:[]ports.CanonicalRecord{event},TriggerRuntime:changes,Activations:activations,
        RevisionCanonical:revisionCanonical,
    },nil
}
func anyIDs(in []domain.ID) []any {out:=make([]any,len(in));for i,id:=range in{out[i]=id[:]};return out}
func encodeRuntime(r trigger.Runtime)([]byte,error){
    last:=[]any{uint64(0)};if r.HasLast{last=[]any{uint64(1),r.Last}}
    next:=[]any{uint64(0)};if r.HasNextRepeat{next=[]any{uint64(1),int64(r.NextRepeat)}}
    return codec.CanonicalPayload([]any{r.DefinitionID[:],uint64(r.Version),last,r.Armed,r.Exhausted,r.ActivationCount,next,uint64(r.EvaluatedThrough)})
}
