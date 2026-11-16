package commit

import (
    "bytes"
    "context"
    "database/sql"
    "encoding/hex"
    "errors"
    "fmt"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
    _ "modernc.org/sqlite"
)

// ReferenceCommunicationProof exposes only rooted identities, clocks and
// cardinalities. Private proposition and knowledge payloads stay in SQLite.
type ReferenceCommunicationProof struct {
    ClaimID           string `json:"claim_id"`
    TransmissionID    string `json:"transmission_id"`
    DeliverySourceID  string `json:"delivery_occurrence_id"`
    DeliveryEventID   string `json:"delivery_event_id"`
    SentAt            int64  `json:"sent_at"`
    DeliveredAt       int64  `json:"delivered_at"`
    BKnowledgeInputs  int    `json:"b_new_knowledge_inputs"`
    ReplayVerified    bool   `json:"replay_verified"`
}

// referenceCompleteC1Tasks is a strict registered-event dispatcher. It never
// constructs Observations/KnowledgeInputs from an untrusted worker string:
// the SQLite projection reads each committed Event/Genesis/owner authority.
// C2 owns independent hostile privacy/owner-checkpoint audit of these results.
func referenceCompleteC1Tasks(ctx context.Context, store *sqlite.Store, bundle *policy.Bundle,
    dbPath string, run domain.ID, through uint64) error {
    db, err := sql.Open("sqlite", "file:"+dbPath)
    if err != nil { return err }
    defer db.Close()
    rows, err := db.QueryContext(ctx, "SELECT id,canonical FROM causal_outbox")
    if err != nil { return err }
    type task struct { id [32]byte; event [32]byte }
    var tasks []task
    for rows.Next() {
        var id, raw []byte
        if err := rows.Scan(&id,&raw); err != nil { rows.Close();return err }
        decoded, err := codec.StrictDecodePayload(raw)
        if err != nil { rows.Close();return err }
        f,ok:=decoded.([]any)
        if !ok {rows.Close();return errors.New("unknown canonical outbox record")}
        // A completion has 5 fields; a pending PerceptionTask has exactly 10.
        if len(f)==5 {continue}
        if len(f)!=10 || len(id)!=32 {rows.Close();return errors.New("unexpected outbox task schema")}
        ev,ok:=f[3].([]byte)
        if !ok||len(ev)!=32 {rows.Close();return errors.New("outbox event ID invalid")}
        var t task
        copy(t.id[:],id);copy(t.event[:],ev)
        tasks=append(tasks,t)
    }
    if err:=rows.Err();err!=nil{rows.Close();return err}
    if err:=rows.Close();err!=nil{return err}
    for _,task:=range tasks {
        var raw []byte
        if err:=db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",task.event[:]).Scan(&raw);err!=nil{return err}
        v,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
        event,ok:=v.([]any)
        if !ok||len(event)!=19 {return errors.New("outbox references malformed Event")}
        kind,ok:=event[3].(string)
        if !ok{return errors.New("unregistered perception Event kind")}
        switch kind {
        case "round.consumed":
            if _,err:=store.DeliverRoundPerception(ctx,task.id,bundle);err!=nil{return fmt.Errorf("addressed round delivery: %w",err)}
        case "communication.delivered":
            if _,err:=store.DeliverCommunicationPerception(ctx,task.id);err!=nil{return fmt.Errorf("addressed communication delivery: %w",err)}
        default:
            if _,err:=store.CompleteInternalPerception(ctx,task.id,bundle);err!=nil{
                return fmt.Errorf("unregistered/visible Event %q cannot be silently completed: %w",kind,err)
            }
        }
    }
    // This is a real read-only durable barrier, not a ceremonial marker.
    return store.CheckPerceptionBarrier(ctx,[32]byte(run),through)
}

func referenceC1BInputs(ctx context.Context,dbPath string)(int,error){
    db,err:=sql.Open("sqlite","file:"+dbPath);if err!=nil{return 0,err}
    defer db.Close()
    var n int
    err=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM evidence_routes WHERE holder_namespace='school' AND holder_id='b' AND kind=1").Scan(&n)
    return n,err
}

func referenceC1Cycle(ctx context.Context,store *sqlite.Store,bundle *policy.Bundle,run domain.ID,
    state clock.State,ordinal domain.CycleOrdinal)(admission.Cycle,error){
    d:=identity.New(bundle)
    id,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
        []any{run[:],[]any{int64(state.Instant),uint64(ordinal)}})
    if err!=nil{return admission.Cycle{},err}
    _,hash,err:=store.LoadWorldState(ctx,uint64(state.Revision))
    if err!=nil{return admission.Cycle{},err}
    return admission.Cycle{ID:domain.ID(id),Attempt:0,Instant:state.Instant,Ordinal:ordinal,
        BaseRevision:state.Revision,BaseStateHash:domain.Digest(hash),RetryRef:[]any{uint64(0)}},nil
}

// extendReferenceC1 continues, rather than recreates, the #116 run. No new
// Genesis, policy authority, stock, actor history or SQLite file is minted.
func extendReferenceC1(ctx context.Context,store *sqlite.Store,ledger *inputledger.Ledger,
    bundle *policy.Bundle,g *genesis.Genesis,c *Coordinator,dbPath string,
    opt ReferenceOptions,out *ReferenceResult)error {
    if err:=referenceCompleteC1Tasks(ctx,store,bundle,dbPath,g.RunID,3);err!=nil{
        return fmt.Errorf("pre-C1 epistemic barrier: %w",err)
    }
    bBefore,err:=referenceC1BInputs(ctx,dbPath);if err!=nil{return err}
    state,err:=store.LoadClock(ctx,[32]byte(g.RunID));if err!=nil{return err}
    if state.Instant!=g.InitialInstant+10||state.Revision!=3||state.LastOrdinal!=0{
        return errors.New("C1 requires completed #116 three-cycle prefix")
    }
    nextOrdinal:=domain.CycleOrdinal(state.LastOrdinal+1)
    at:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant,NotBeforeCycleOrdinal:nextOrdinal}
    origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
    round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","a"},[]string{"act"},at,
        origin,0,2,0,[]any{uint64(0)})
    if err!=nil{return fmt.Errorf("C1 send RoundDeclaration: %w",err)}
    sendCycle,err:=referenceC1Cycle(ctx,store,bundle,g.RunID,state,nextOrdinal);if err!=nil{return err}
    d:=identity.New(bundle)
    dispatchID,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
        []any{g.RunID[:],sendCycle.ID[:],round.ID[:],"act",uint64(0)})
    if err!=nil{return err}
    dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchID),RunID:g.RunID,CycleID:sendCycle.ID,RoundID:round.ID,
        SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,BaseRevision:state.Revision,EffectiveAt:state.Instant}
    if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
        ID:sendCycle.ID,BaseRevision:state.Revision,Instant:state.Instant});err!=nil{return fmt.Errorf("C1 guarded dispatch: %w",err)}
    proposal,err:=testresourcev1.ProposalForDispatch(bundle,dispatch);if err!=nil{return err}
    scope,err:=communication.MakeTyped("cote.csf.communication.scope.v1","cote.csf.schema.typed-value",
        []any{uint64(0)});if err!=nil{return err}
    alleged,err:=communication.MakeTyped("cote.csf.communication.allegation.v1","cote.csf.schema.typed-value",
        []any{"resource.stolen",[]any{"school","c"},"last.resource"});if err!=nil{return err}
    due:=state.Instant+25
    params,err:=communication.SendParameters(communication.SendSpec{
        Recipient:communication.Actor{"school","b"},PresentedSender:"C",
        Channel:"school.message.v1",DueAt:due,
        Proposition:communication.Proposition{Type:"resource.stolen",Polarity:0,
            TemporalScope:scope,Content:alleged}})
    if err!=nil{return err}
    proposal.Body[4]="communication.send"
    proposal.Body[5]=uint64(1)
    proposal.Body[7]=[]any{}
    proposal.Body[8]=params
    proposal.Body[9]=[]any{}
    proposal.Body[10]=[]any{uint64(0)}
    if opt.ForgeCommunicationActor {
        proposal.Body[6]=[]any{"school","c"}
    }
    digest,_,err:=d.Sum("cote.csf.digest.action-proposal-unit",
        "cote.csf.schema.action-proposal.body",1,proposal.Body)
    if err!=nil{return err}
    proposal.Digest=digest
    proposal.Canonical,err=codec.CanonicalPayload(append(cloneAny(proposal.Body),digest[:]))
    if err!=nil{return err}
    if err:=ledger.Respond(ctx,proposal);err!=nil{return fmt.Errorf("C1 authenticated SlotResponse: %w",err)}
    sendFence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
        ctx,bundle,ledger,sendCycle,c.admissionPolicies,nil)
    if err!=nil{return err}
    if units,ok:=sendFence.Fields[11].([]any);!ok||len(units)!=1{
        return errors.New("C1 refused a non-singleton communication send cohort")
    }
    c.failAfterStep=opt.FailCommunicationSendAt
    sent,sendErr:=c.CommitCommunicationSend(ctx,ledger,sendCycle,sendFence)
    c.failAfterStep=0
    if sendErr!=nil{return fmt.Errorf("C1 communication send: %w",sendErr)}
    stateSent,err:=store.LoadClock(ctx,[32]byte(g.RunID));if err!=nil{return err}
    if stateSent.Revision!=4||stateSent.LastOrdinal!=int64(nextOrdinal){return errors.New("C1 message was not terminally committed")}
    _,sentHash,err:=store.LoadWorldState(ctx,uint64(stateSent.Revision));if err!=nil{return err}
    _,priorHash,err:=store.LoadWorldState(ctx,3);if err!=nil{return err}
    if sentHash!=priorHash{
        return errors.New("C1 false message changed World Truth")
    }
    if err:=referenceCompleteC1Tasks(ctx,store,bundle,dbPath,g.RunID,4);err!=nil{return fmt.Errorf("C1 send barrier: %w",err)}
    bAfterSend,err:=referenceC1BInputs(ctx,dbPath);if err!=nil{return err}
    if bAfterSend!=bBefore{return errors.New("B received claim KnowledgeInput before due delivery")}
    advance,err:=c.Advance(ctx,ledger,g.RunID);if err!=nil{return fmt.Errorf("C1 due clock advance: %w",err)}
    if advance.Plan.Kind!=clock.ClockAdvance||advance.Plan.To!=due{
        return errors.New("C1 delivery was not scheduled at its pinned causal instant")
    }
    afterAdvance:=advance.ResultClock
    if err:=referenceCompleteC1Tasks(ctx,store,bundle,dbPath,g.RunID,uint64(afterAdvance.Revision));err!=nil{return err}
    bBeforeDelivery,err:=referenceC1BInputs(ctx,dbPath);if err!=nil{return err}
    if bBeforeDelivery!=bBefore{return errors.New("B observed in-transit Claim during clock advance")}
    pending,err:=store.PendingClock(ctx,[32]byte(g.RunID));if err!=nil{return err}
    plan,ok,err:=clock.Next(afterAdvance,pending)
    if err!=nil||!ok||plan.Kind!=clock.Work||plan.Target.NotBeforeInstant!=due{
        return fmt.Errorf("C1 no due communication WORK: %v",err)
    }
    dueCycle,err:=referenceC1Cycle(ctx,store,bundle,g.RunID,afterAdvance,plan.OriginOrdinal);if err!=nil{return err}
    dueFence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
        ctx,bundle,ledger,dueCycle,c.admissionPolicies,nil)
    if err!=nil{return err}
    if units,ok:=dueFence.Fields[11].([]any);!ok||len(units)!=1||units[0].([]any)[1]!=uint64(2){
        return errors.New("C1 due fence admitted another source")
    }
    c.failAfterStep=opt.FailCommunicationDeliveryAt
    delivered,deliveryErr:=c.CommitCommunicationDue(ctx,ledger,dueCycle,dueFence)
    c.failAfterStep=0
    if deliveryErr!=nil{return fmt.Errorf("C1 due commit: %w",deliveryErr)}
    if _,err:=c.CommitCommunicationDue(ctx,ledger,dueCycle,dueFence);err==nil{
        return errors.New("C1 re-delivered already terminal occurrence")
    }
    stateFinal,err:=store.LoadClock(ctx,[32]byte(g.RunID));if err!=nil{return err}
    if stateFinal.Revision!=6||stateFinal.Instant!=due{return errors.New("C1 terminal clock coordinate invalid")}
    world,finalHash,err:=store.LoadWorldState(ctx,6);if err!=nil{return err}
    stock,err:=reducer.DecodeStock(world);if err!=nil||stock.Available!=0||finalHash!=sentHash{
        return errors.New("C1 claim/delivery changed committed inventory")
    }
    if err:=referenceCompleteC1Tasks(ctx,store,bundle,dbPath,g.RunID,6);err!=nil{return err}
    bAfter,err:=referenceC1BInputs(ctx,dbPath);if err!=nil{return err}
    if bAfter!=bBefore+1{return fmt.Errorf("C1 B KnowledgeInput delta %d, expected 1",bAfter-bBefore)}
    replay,err:=store.ReplayClock(ctx,[32]byte(g.RunID),bundle)
    if err!=nil{return err}
    if replay.ReconstructedClock!=stateFinal||replay.ReplayedCommits!=6||replay.VerifiedActivations!=1{
        return errors.New("C1 integrated causal replay mismatch")
    }
    db,err:=sql.Open("sqlite","file:"+dbPath);if err!=nil{return err}
    defer db.Close()
    var claimID,txID,sourceID []byte
    var status int
    if err:=db.QueryRowContext(ctx,"SELECT id FROM communication_claims").Scan(&claimID);err!=nil{return err}
    if err:=db.QueryRowContext(ctx,"SELECT id,occurrence_id,status FROM communication_transmissions").Scan(&txID,&sourceID,&status);err!=nil{return err}
    if status!=1||len(claimID)!=32||len(txID)!=32||len(sourceID)!=32{return errors.New("C1 root/terminal transmission invalid")}
    var eventID []byte
    rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM event_store");if err!=nil{return err}
    var deliveryCount int
    for rows.Next(){
        var id,raw []byte
        if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return err}
        decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return err}
        v,ok:=decoded.([]any);if !ok||len(v)!=19{rows.Close();return errors.New("C1 malformed EventStore item")}
        if v[3]=="resource.stolen"{rows.Close();return errors.New("C1 false Claim became World Event")}
        if v[3]=="communication.delivered"{eventID=bytes.Clone(id);deliveryCount++}
    }
    if err:=rows.Err();err!=nil{rows.Close();return err}
    if err:=rows.Close();err!=nil{return err}
    if deliveryCount!=1{return errors.New("C1 delivery Event not unique")}
    events,err:=referenceEventBytes(ctx,dbPath);if err!=nil{return err}
    rawDigest:=identity.OpaqueContentHash(events)
    out.Status="RUN_LOCAL_10C1"
    out.Cycles=append(out.Cycles,referenceCycle("COMMUNICATION_SEND",sendCycle,sendFence,sent),
        referenceCycle("COMMUNICATION_CLOCK_ADVANCE",admission.Cycle{Instant:advance.Plan.From,Ordinal:advance.Plan.OriginOrdinal},advance.Fence,advance.Commit),
        referenceCycle("COMMUNICATION_DELIVERY",dueCycle,dueFence,delivered))
    out.CycleFenceBytes=append(out.CycleFenceBytes,bytes.Clone(sendFence.Canonical),bytes.Clone(advance.Fence.Canonical),bytes.Clone(dueFence.Canonical))
    out.CycleCommitBytes=append(out.CycleCommitBytes,bytes.Clone(sent.Canonical),bytes.Clone(advance.Commit.Canonical),bytes.Clone(delivered.Canonical))
    out.FinalRevision=6;out.FinalInstant=int64(due);out.FinalWorldHash=hex.EncodeToString(finalHash[:])
    out.ReplayedCommits=uint64(replay.ReplayedCommits);out.ReplayedEvents=uint64(replay.ReplayedEvents)
    out.EventBytes=events;out.EventStreamSHA256=hex.EncodeToString(rawDigest[:])
    out.Communication=&ReferenceCommunicationProof{
        ClaimID:hex.EncodeToString(claimID),TransmissionID:hex.EncodeToString(txID),
        DeliverySourceID:hex.EncodeToString(sourceID),DeliveryEventID:hex.EncodeToString(eventID),
        SentAt:int64(state.Instant),DeliveredAt:int64(due),
        BKnowledgeInputs:bAfter-bBefore,ReplayVerified:true}
    return nil
}
