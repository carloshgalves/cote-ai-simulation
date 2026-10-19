// Package commit provides the only CLOCK_ADVANCE builder and transaction caller.
// The WORK/settlement branch will be added by CSFRV1-4/5; it is NOT a second
// publication authority.
package commit

import (
    "bytes"
    "context"
    "sort"
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
    LoadRegisteredTriggers(context.Context,[32]byte,*policy.Bundle)([]trigger.Registered,error)
    LoadGenesis(context.Context,[32]byte)([]byte,error)
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
    // Fault injection is deliberately unexported; only same-package tests
    // can interrupt a SQLite transaction before its commit.
    failAfterStep uint8
    // Test-only interleaving hook; production coordinator has no callback.
    beforeCommit func()
    // Same-package tests may submit damaged bytes to the durable adapter.
    tamperBatch func(*ports.ClockAdvanceBatch)
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
func bodyValue(from,to domain.SimulationInstant)([]any,error){
    // Temporal fixture only: not a physical reducer or Embodiment update.
    bytes,err:=codec.CanonicalBytes("cote.csf.body.synthetic-advance.v1","cote.csf.schema.typed-value",1,[]any{int64(from),int64(to)})
    if err!=nil{return nil,err}
    digest:=identity.OpaqueContentHash(bytes)
    return []any{"cote.csf.schema.typed-value",uint64(1),bytes,digest[:]},nil
}

func bytes32(v any)([32]byte,error){
    var dst [32]byte
    b,ok:=v.([]byte);if !ok||len(b)!=32{return dst,errors.New("expected a pinned 32-byte digest")}
    copy(dst[:],b);return dst,nil
}
func cloneAny(x []any)[]any {v:=make([]any,len(x));copy(v,x);return v}

func (c *Coordinator) Advance(ctx context.Context,ledger *inputledger.Ledger,runID domain.ID)(Outcome,error){
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
        prepared,err:=c.buildBatch(ctx,runID,state,plan,f,typed)
        if err!=nil{return err}
        prepared.FailAfterStep=c.failAfterStep
        if c.beforeCommit!=nil {c.beforeCommit()}
        if c.tamperBatch!=nil {c.tamperBatch(&prepared)}
        applied=prepared
        return c.store.CommitClockAdvance(ctx,prepared)
    })
    if err!=nil{return Outcome{},err}
    after,err:=clock.Decode(applied.ResultClock);if err!=nil{return Outcome{},err}
    return Outcome{Plan:plan,Fence:fence,Commit:applied.Journal,Events:applied.Events,Activations:applied.Activations,ResultClock:after},nil
}

func (c *Coordinator) buildBatch(ctx context.Context,runID domain.ID,state clock.State,plan clock.Plan,f admission.Fence,baseTyped []any)(ports.ClockAdvanceBatch,error){
    var empty ports.ClockAdvanceBatch
    d:=identity.New(c.bundle)
    next,err:=clock.ApplyAdvance(state,plan);if err!=nil{return empty,err}
    beforeBytes,err:=clock.Encode(state);if err!=nil{return empty,err}
    afterBytes,err:=clock.Encode(next);if err!=nil{return empty,err}
    origin,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,[]any{runID[:],[]any{int64(plan.From),uint64(plan.OriginOrdinal)}})
    if err!=nil{return empty,err}
    // Versioned synthetic time and body passage share this one commit.
    // Both are explicitly not perceptible (privacy=2, tasks=[]).
    events:=make([]ports.CanonicalRecord,0,2)
    preTriggerEvents:=make([]trigger.Event,0,2)
    eventIDs:=make([]any,0,2)
    canonicalEvents:=make([]any,0,2)
    for index,kind:=range []string{"clock.advanced","body.advanced"}{
        key:=[]any{uint64(0),origin[:],"clock",uint64(1),uint64(index),uint64(0)}
        id,_,err:=d.Sum("cote.csf.id.event","cote.csf.schema.event-id-preimage",1,[]any{runID[:],origin[:],key})
        if err!=nil{return empty,err}
        var value []any
        if kind=="clock.advanced" {value,err=eventValue(plan.From,plan.To)
        } else {value,err=bodyValue(plan.From,plan.To)}
        if err!=nil{return empty,err}
        body:=[]any{
            id[:],runID[:],key,kind,uint64(1),int64(plan.From),
            uint64(state.NextLogicalSequence)+uint64(index),origin[:],uint64(state.Revision),value,
            []any{},[]any{},[]any{},[]any{},[]any{uint64(0)},uint64(2),"clock",uint64(1),
        }
        digest,_,err:=d.Sum("cote.csf.digest.event","cote.csf.schema.event.body",1,body)
        if err!=nil{return empty,err}
        canonical,err:=codec.CanonicalPayload(append(cloneAny(body),digest[:]));if err!=nil{return empty,err}
        events=append(events,ports.CanonicalRecord{Identity:[32]byte(id),Canonical:canonical})
        eventIDs=append(eventIDs,id[:])
        canonicalEvents=append(canonicalEvents,canonical)
        footprint,err:=trigger.EventFootprint(kind);if err!=nil{return empty,err}
        keyBytes,err:=codec.CanonicalPayload(key);if err!=nil{return empty,err}
        preTriggerEvents=append(preTriggerEvents,trigger.Event{ID:domain.ID(id),OrderKey:keyBytes,Footprint:footprint})
    }
    canonicalState,ok:=baseTyped[2].([]byte);if !ok{return empty,errors.New("genesis state envelope malformed")}
    stateDigest,err:=bytes32(baseTyped[3]);if err!=nil{return empty,err}
    revisionCanonical,err:=codec.CanonicalPayload([]any{uint64(state.Revision),baseTyped,stateDigest[:],uint64(next.Revision),baseTyped,stateDigest[:]})
    if err!=nil{return empty,err}
    inputs,err:=c.store.Records(ctx);if err!=nil{return empty,err}
    _=canonicalState
    changes:=make([]ports.TriggerStateChange,0)
    activations:=make([]ports.CanonicalRecord,0)
    ordered,err:=c.store.LoadRegisteredTriggers(ctx,[32]byte(runID),c.bundle)
    if err!=nil{return empty,err}
    sort.Slice(ordered,func(i,j int)bool{return bytes.Compare(ordered[i].Definition.ID[:],ordered[j].Definition.ID[:])<0})
    for i,item:=range ordered {
        if i>0 && item.Definition.ID==ordered[i-1].Definition.ID {
            return empty,errors.New("duplicate trigger definition in one cycle")
        }
        predicateResult:=trigger.PredicateValue(item.Definition,next.Instant)
        fired,err:=trigger.Evaluate(item.Definition,item.Runtime,next.Instant,next.Revision,predicateResult,preTriggerEvents)
        if err!=nil{return empty,err}
        runtime:=fired.Runtime
        prior:=item.RuntimeCanonical
        after,err:=trigger.RuntimeBytes(runtime);if err!=nil{return empty,err}
        changes=append(changes,ports.TriggerStateChange{DefinitionID:[32]byte(item.Definition.ID),DefinitionCanonical:item.DefinitionCanonical,Before:prior,After:after})
        if !fired.Fired{continue}
         activationID,raw,err:=trigger.CycleActivation(c.bundle,runID,item.Definition,runtime,domain.ID(origin),next.Revision,next.Instant,fired.CausingEventIDs)
         if err!=nil{return empty,err}
activations=append(activations,ports.CanonicalRecord{Identity:[32]byte(activationID),Canonical:raw})
    }
    // Trigger state transitions and newly-created activations are internal
    // lifecycle facts in the SAME terminal batch; only the two temporal
    // events participate in pre-trigger footprint evaluation.
    sequence:=state.NextLogicalSequence+2
    for _,change:=range changes{
        delta,err:=codec.CanonicalPayload([]any{change.DefinitionID[:],change.Before,change.After})
        if err!=nil{return empty,err}
        id,raw,err:=trigger.LifecycleEvent(c.bundle,runID,domain.ID(origin),state.Instant,state.Revision,
            sequence,3,domain.ID(change.DefinitionID),delta)
        if err!=nil{return empty,err}
        events=append(events,ports.CanonicalRecord{Identity:[32]byte(id),Canonical:raw})
        eventIDs=append(eventIDs,id[:]);canonicalEvents=append(canonicalEvents,raw);sequence++
    }
    // Phase 4 is ordered by EventOrderKey origin_ref = activation identity.
    sort.Slice(activations,func(i,j int)bool{return bytes.Compare(activations[i].Identity[:],activations[j].Identity[:])<0})
    for _,activation:=range activations{
        id,raw,err:=trigger.LifecycleEvent(c.bundle,runID,domain.ID(origin),state.Instant,state.Revision,
            sequence,4,domain.ID(activation.Identity),activation.Canonical)
        if err!=nil{return empty,err}
        events=append(events,ports.CanonicalRecord{Identity:[32]byte(id),Canonical:raw})
        eventIDs=append(eventIDs,id[:]);canonicalEvents=append(canonicalEvents,raw);sequence++
    }
    next.NextLogicalSequence=sequence
    afterBytes,err=clock.Encode(next);if err!=nil{return empty,err}
    noDecisions:=[]any{}
    decisionDigest,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,noDecisions)
    if err!=nil{return empty,err}
    taskDigest,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,[]any{})
    if err!=nil{return empty,err}
    policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
    planBytes,err:=clock.EncodePlan(plan);if err!=nil{return empty,err}
    commitBody:=[]any{
        runID[:],origin[:],uint64(0),int64(state.Instant),uint64(plan.OriginOrdinal),planBytes,
        uint64(state.Revision),f.Fields[8],uint64(next.Revision),f.Fields[8],
        f.Digest[:],[]any{},f.InputDigest[:],policies["cycle-coordinate"],policies["event-order"],
        eventIDs,[]any{},decisionDigest[:],[]any{},taskDigest[:],
        policies["perception"],policies["perception-identity"],uint64(next.NextLogicalSequence),
    }
    batchDigest,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,[]any{commitBody,canonicalEvents})
    if err!=nil{return empty,err}
    journalBytes,err:=codec.CanonicalPayload(append(cloneAny(commitBody),batchDigest[:]));if err!=nil{return empty,err}
    commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,[]any{runID[:],origin[:],uint64(0)})
    if err!=nil{return empty,err}
    return ports.ClockAdvanceBatch{
        RunID:[32]byte(runID),CycleID:[32]byte(origin),FenceID:f.ID,
        TargetInstant:int64(plan.To),TargetOrdinal:uint64(plan.Target.NotBeforeCycleOrdinal),
        ExpectedInputPrefix:inputs,ExpectedRevision:uint64(state.Revision),ResultRevision:uint64(next.Revision),
        ExpectedClock:beforeBytes,ResultClock:afterBytes,
        BaseStateHash:stateDigest,ResultStateHash:stateDigest,
        Journal:ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalBytes},
        Events:events,TriggerRuntime:changes,Activations:activations,
        RevisionCanonical:revisionCanonical,
    },nil
}
func anyIDs(in []domain.ID) []any {out:=make([]any,len(in));for i,id:=range in{out[i]=id[:]};return out}
