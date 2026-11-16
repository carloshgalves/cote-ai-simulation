package commit

import (
    "bytes"
    "context"
    "encoding/hex"
    "errors"
    "fmt"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

// ReferenceC4Proof attests combined C1, C2 and C3 *in the same* RunID,
// SQLite and ledger prefixes. It is an internal test report: the public
// output is a separate strict allowlist, never a re-marshal of ReferenceResult.
type ReferenceC4Proof struct {
    LedgerDigests map[string]string `json:"ledger_digests"`
    LedgerCounts map[string]int `json:"ledger_counts"`
    SnapshotAndReplayVerified bool `json:"snapshot_and_replay_verified"`
    FiveLedgersVerified bool `json:"five_ledgers_verified"`
    OutboxAndScheduleVerified bool `json:"outbox_and_schedule_verified"`
    ClaimNotWorldTruth bool `json:"claim_not_world_truth"`
    OwnerAndPOVVerified bool `json:"owner_and_pov_verified"`
    ReportRedacted bool `json:"report_redacted"`
}

// All six causal cycles happen once. No simulation copies, secondary
// Genesis, separate ABORT-run database, or claimant-authored owner evidence.
func RunReferenceC4(ctx context.Context,dbPath,ownerPath,bundlePath string,
    options ReferenceOptions)(ReferenceResult,error){
    var zero ReferenceResult
    if ctx==nil{return zero,errors.New("C4 requires an execution context")}
    options.IncludeCommunication=true
    options.IncludeAbortRetry=true
    // RunReferenceC2 already executes real B+C1 in one SQLite, stages both
    // A/B signed private owner stores, reopens, ACKs, verifies the causal
    // snapshot and enforces holder-only POV. It now accepts a *real* C3
    // authorized retry before the same C1 six-cycle continuation.
    result,err:=RunReferenceC2(ctx,dbPath,ownerPath,bundlePath,options)
    if err!=nil{return zero,err}
    if result.Status!="RUN_LOCAL_10C2"||result.AbortRetry==nil||
        result.Communication==nil||result.Epistemic==nil||
        len(result.Cycles)!=6||result.FinalRevision!=6||
        result.FinalInstant!=35||result.FinalStock!=0||
        result.ReplayedCommits!=6{
        return zero,errors.New("C4 refused a C1/C2/C3 run with missing real causal authority")
    }
    c3,c1,c2:=result.AbortRetry,result.Communication,result.Epistemic
    if !c3.ValidatorIndeterminate||c3.IndeterminateSubjects!=2||
        !c3.HaltedAfterRestart||!c3.UnauthorizedFenceDenied||
        !c3.RetryAuthorizedAfterRestart||!c3.SameMembership||
        !c3.SameInputDigest||!c3.SamePolicyAndClock||
        c3.AbortRevision!=0||c3.AbortEvents!=0||c3.AbortCommitCount!=0||
        c3.RetryAttempt!=1||c1.SentAt!=10||c1.DeliveredAt!=35||
        c1.BKnowledgeInputs!=1||!c1.ReplayVerified||
        c2.BReceivedAt!=35||c2.BKnowledgeInputs!=1||
        !c2.POVIsolated||!c2.BarrierVerified||!c2.CompletionIdempotent||
        !c2.WorkDeniedBeforeACK||!c2.RestoredAfterReopen||
        !c2.SeparateOwnerKeys||c2.SnapshotID==""||
        c2.OwnerReceiptID==c2.AOwnerReceiptID{
        return zero,errors.New("C4 cross-slice witness mismatch")
    }
    bundle,err:=policy.Load(bundlePath)
    if err!=nil{return zero,err}
    runID,err:=referenceC4DecodeID(result.RunID)
    if err!=nil{return zero,err}
    store,err:=sqlite.Open(ctx,dbPath)
    if err!=nil{return zero,err}
    defer store.Close()
    if err:=store.CheckPerceptionBarrier(ctx,runID,6);err!=nil{
        return zero,fmt.Errorf("C4 restored perception barrier: %w",err)
    }
    snapshotID,err:=referenceC4DecodeID(c2.SnapshotID)
    if err!=nil{return zero,err}
    snapshot,err:=store.VerifySnapshotV1(ctx,runID,snapshotID,bundle)
    if err!=nil||snapshot.Snapshot.ID!=domain.ID(snapshotID)||snapshot.TailCommits!=0{
        return zero,fmt.Errorf("C4 five-ledger+owner snapshot verification: %w",err)
    }
    replay,err:=store.ReplayClock(ctx,runID,bundle)
    if err!=nil||replay.ReplayedCommits!=6||
        replay.ReplayedEvents!=int(result.ReplayedEvents)||
        replay.ReconstructedClock.Revision!=6||
        replay.ReconstructedClock.Instant!=35{
        return zero,fmt.Errorf("C4 full after-reopen historical replay failed: %w",err)
    }
    // C4 scans every persisted canonical byte from the five normatively
    // sealed ledgers, rather than trusting booleans calculated by C1/C2/C3.
    digests,counts,err:=referenceC4FiveLedgers(ctx,dbPath,bundle)
    if err!=nil{return zero,err}
    if len(digests)!=5||len(counts)!=5{
        return zero,errors.New("C4 missing one of five canonical ledger prefixes")
    }
    // Bind each independently recalculated prefix to the exact canonical
    // sealed snapshot, not merely to a count/digest created by this audit.
    snapVal,err:=codec.StrictDecodePayload(snapshot.Snapshot.Canonical)
    if err!=nil{return zero,err}
    snapFields,ok:=snapVal.([]any)
    if !ok||len(snapFields)!=19{return zero,errors.New("C4 snapshot schema malformed")}
    cursors,ok:=snapFields[12].([]any)
    if !ok||len(cursors)!=5{return zero,errors.New("C4 five-ledger snapshot cursor missing")}
    for i,name:=range referenceC4LedgerNames{
        cursor,ok:=cursors[i].([]any)
        if !ok||len(cursor)!=3||cursor[0]!=name{
            return zero,fmt.Errorf("C4 snapshot cursor %s not present in expected slot",name)
        }
        sealedCount,ok:=cursor[1].(uint64)
        if !ok||int(sealedCount)!=counts[name]{
            return zero,fmt.Errorf("C4 %s count does not equal sealed snapshot prefix",name)
        }
        sealedDigest,ok:=cursor[2].([]byte)
        if !ok||hex.EncodeToString(sealedDigest)!=digests[name]{
            return zero,fmt.Errorf("C4 %s digest differs from authenticated snapshot",name)
        }
    }
    for name,count:=range counts{
        if count==0||len(digests[name])!=64{
            return zero,fmt.Errorf("C4 empty or malformed %s ledger prefix",name)
        }
    }
    db,err:=openReferenceReadOnlyDB(dbPath)
    if err!=nil{return zero,err}
    defer db.Close()
    claimID,err:=referenceC4DecodeID(c1.ClaimID)
    if err!=nil{return zero,err}
    txID,err:=referenceC4DecodeID(c1.TransmissionID)
    if err!=nil{return zero,err}
    taskID,err:=referenceC4DecodeID(c1.DeliveryTaskID)
    if err!=nil{return zero,err}
    var transmissionStatus,scheduledLifecycle int
    var sourceID,storedClaim []byte
    if err:=db.QueryRowContext(ctx,
        "SELECT status,occurrence_id,claim_id FROM communication_transmissions WHERE id=? AND run_id=?",
        txID[:],runID[:]).Scan(&transmissionStatus,&sourceID,&storedClaim);err!=nil{return zero,err}
    if transmissionStatus!=1||len(sourceID)!=32||!bytes.Equal(storedClaim,claimID[:]){
        return zero,errors.New("C4 transmission not delivered or Claim reference swapped")
    }
    if err:=db.QueryRowContext(ctx,
        "SELECT lifecycle FROM scheduled_occurrences WHERE id=? AND run_id=?",
        sourceID,runID[:]).Scan(&scheduledLifecycle);err!=nil{return zero,err}
    // scheduled_occurrences.lifecycle is immutable BIRTH state, not a
    // mutable consumption flag. The C1 due commit and terminal transmission
    // settle it; ReplayClock plus snapshotPending independently exclude
    // consumed occurrences from the live pending cohort.
    if scheduledLifecycle!=0{
        return zero,errors.New("C4 scheduled occurrence birth index was altered")
    }
    pending,err:=store.PendingClock(ctx,runID)
    if err!=nil{return zero,err}
    for _,unit:=range pending{
        if bytes.Equal(unit.ID[:],sourceID){
            return zero,errors.New("C4 delivered occurrence still belongs to live pending cohort")
        }
    }
    var evidenceLinks int
    if err:=db.QueryRowContext(ctx,
        "SELECT COUNT(*) FROM evidence_routes WHERE run_id=? AND task_id=? AND holder_namespace='school' AND holder_id='b'",
        runID[:],taskID[:]).Scan(&evidenceLinks);err!=nil{return zero,err}
    if evidenceLinks!=2{return zero,errors.New("C4 B-only Observation/KnowledgeInput pair is not exact")}
    var alienLinks int
    if err:=db.QueryRowContext(ctx,
        "SELECT COUNT(*) FROM evidence_routes WHERE task_id=? AND (run_id<>? OR holder_namespace<>'school' OR holder_id<>'b')",
        taskID[:],runID[:]).Scan(&alienLinks);err!=nil{return zero,err}
    if alienLinks!=0{return zero,errors.New("C4 delivered claim leaked to foreign holder")}
    deliveryID,err:=referenceC4DecodeID(c1.DeliveryEventID)
    if err!=nil{return zero,err}
    actualTask,actualKnowledge,err:=referenceC1DeliveryEvidence(
        ctx,dbPath,domain.ID(runID),deliveryID[:])
    if err!=nil{return zero,fmt.Errorf("C4 delivered task lacks registered causal evidence: %w",err)}
    if actualTask!=c1.DeliveryTaskID||actualKnowledge!=c1.KnowledgeInputID{
        return zero,errors.New("C4 delivery/event/task/knowledge identity mismatch")
    }
    rawRows,err:=db.QueryContext(ctx,"SELECT canonical FROM event_store")
    if err!=nil{return zero,err}
    sent,delivered:=0,0
    for rawRows.Next(){
        var raw []byte
        if err:=rawRows.Scan(&raw);err!=nil{rawRows.Close();return zero,err}
        v,err:=codec.StrictDecodePayload(raw)
        if err!=nil{rawRows.Close();return zero,err}
        fields,ok:=v.([]any)
        if !ok||len(fields)!=19{rawRows.Close();return zero,errors.New("C4 event schema malformed")}
        switch fields[3]{
        case "resource.stolen":
            rawRows.Close();return zero,errors.New("C4 false claim promoted to World Truth")
        case "communication.sent":sent++
        case "communication.delivered":delivered++
        }
    }
    if err:=rawRows.Err();err!=nil{rawRows.Close();return zero,err}
    if err:=rawRows.Close();err!=nil{return zero,err}
    if sent!=1||delivered!=1{return zero,errors.New("C4 send/delivery not unique")}
    // False allegations and messages must not change World Truth. Inspect
    // actual WorldRevision bytes for the full C1 chain (3..6), not merely
    // claim absence from EventStore or a final stock result.
    var settledHash [32]byte
    for revision:=uint64(3);revision<=6;revision++{
        state,digest,err:=store.LoadWorldState(ctx,revision)
        if err!=nil{return zero,err}
        stock,err:=reducer.DecodeStock(state)
        if err!=nil||stock.Available!=0{
            return zero,errors.New("C4 communication changed resource state")
        }
        if revision==3{settledHash=digest}else if settledHash!=digest{
            return zero,errors.New("C4 Claim/send/delivery changed WorldState hash")
        }
    }
    if hex.EncodeToString(settledHash[:])!=result.FinalWorldHash||
        result.FinalWorldHash!=result.ContestWorldHash{
        return zero,errors.New("C4 final world hash does not match contest settlement")
    }
    result.Status="RUN_LOCAL_10C4"
    result.CrossAcceptance=&ReferenceC4Proof{
        LedgerDigests:digests,LedgerCounts:counts,
        SnapshotAndReplayVerified:true,FiveLedgersVerified:true,
        OutboxAndScheduleVerified:true,ClaimNotWorldTruth:true,
        OwnerAndPOVVerified:true,
    }
    // Explicit allowlist is the only public artifact. Neither the private
    // candidate/fence bytes nor Owner content and signed grant payload may be
    // serialized by the C4 CLI.
    public,err:=result.C4PublicReport()
    if err!=nil{return zero,err}
    if public.Status!="RUN_LOCAL_10C4"||public.RunID!=result.RunID{
        return zero,errors.New("C4 public report verification failed")
    }
    result.CrossAcceptance.ReportRedacted=true
    return result,nil
}
func referenceC4DecodeID(encoded string)([32]byte,error){
    var id [32]byte
    value,err:=hex.DecodeString(encoded)
    if err!=nil||len(value)!=32{return id,errors.New("C4 invalid canonical 32-byte root")}
    copy(id[:],value)
    return id,nil
}
var referenceC4LedgerNames=[]string{
    "input_ledger","decision_ledger","event_store","evidence_ledger","causal_outbox",
}
func referenceC4FiveLedgers(ctx context.Context,path string,b *policy.Bundle)(
    map[string]string,map[string]int,error){
    db,err:=openReferenceReadOnlyDB(path)
    if err!=nil{return nil,nil,err}
    defer db.Close()
    d:=identity.New(b)
    digests:=make(map[string]string,5)
    counts:=make(map[string]int,5)
    for _,name:=range referenceC4LedgerNames{
        rows,err:=db.QueryContext(ctx,"SELECT canonical FROM "+name+" ORDER BY id")
        if err!=nil{return nil,nil,err}
        records:=make([]any,0)
        for rows.Next(){
            var raw []byte
            if err:=rows.Scan(&raw);err!=nil{rows.Close();return nil,nil,err}
            if len(raw)==0{rows.Close();return nil,nil,errors.New("empty authoritative ledger record")}
            // Fail closed on both malformed and noncanonical persisted bytes.
            if _,err:=codec.StrictDecodePayload(raw);err!=nil{
                rows.Close();return nil,nil,fmt.Errorf("C4 %s has invalid canonical CBOR: %w",name,err)
            }
            records=append(records,bytes.Clone(raw))
        }
        if err:=rows.Err();err!=nil{rows.Close();return nil,nil,err}
        if err:=rows.Close();err!=nil{return nil,nil,err}
        digest,_,err:=d.Sum("cote.csf.digest.ledger-prefix",
            "cote.csf.schema.ledger-prefix-digest-preimage",1,[]any{name,records})
        if err!=nil{return nil,nil,err}
        counts[name]=len(records)
        digests[name]=hex.EncodeToString(digest[:])
    }
    return digests,counts,nil
}

// Public C4 reporting is intentionally separate from ReferenceResult, whose
// internal diagnostic fields legitimately contain privileged resource state.
type ReferenceC4PublicReport struct {
    Status string `json:"status"`
    RunID string `json:"run_id"`
    CommitCount int `json:"commit_count"`
    Delivery ReferenceC4Delivery `json:"delivery"`
    Owner ReferenceC4Owner `json:"owner"`
    Retry ReferenceC4Retry `json:"retry"`
    Integrity ReferenceC4Integrity `json:"integrity"`
}
type ReferenceC4Delivery struct{
    ClaimID string `json:"claim_id"`
    TransmissionID string `json:"transmission_id"`
    DeliveryTaskID string `json:"delivery_task_id"`
    ReceivedAt int64 `json:"received_at"`
}
type ReferenceC4Owner struct{
    SnapshotID string `json:"snapshot_id"`
    AReceiptID string `json:"a_receipt_id"`
    BReceiptID string `json:"b_receipt_id"`
    RestoredAfterReopen bool `json:"restored_after_reopen"`
    POVIsolated bool `json:"pov_isolated"`
}
type ReferenceC4Retry struct{
    AbortID string `json:"abort_id"`
    RetryID string `json:"retry_id"`
    ValidatorIndeterminate bool `json:"validator_indeterminate"`
    AuthorizedAfterReopen bool `json:"authorized_after_reopen"`
}
type ReferenceC4Integrity struct{
    LedgerDigests map[string]string `json:"ledger_digests"`
    LedgerCounts map[string]int `json:"ledger_counts"`
    FiveLedgersVerified bool `json:"five_ledgers_verified"`
    OutboxAndScheduleVerified bool `json:"outbox_and_schedule_verified"`
    SnapshotAndReplayVerified bool `json:"snapshot_and_replay_verified"`
    ClaimNotWorldTruth bool `json:"claim_not_world_truth"`
}
func (r ReferenceResult) C4PublicReport()(ReferenceC4PublicReport,error){
    var zero ReferenceC4PublicReport
    if r.Status!="RUN_LOCAL_10C4"||len(r.Cycles)!=6||r.FinalRevision!=6||
        r.Communication==nil||r.Epistemic==nil||r.AbortRetry==nil||
        r.CrossAcceptance==nil{
        return zero,errors.New("C4 public report cannot be published from an unverified partial run")
    }
    a,b,c:=r.Communication,r.Epistemic,r.AbortRetry
    p:=r.CrossAcceptance
    if !p.SnapshotAndReplayVerified||!p.FiveLedgersVerified||
        !p.OutboxAndScheduleVerified||!p.ClaimNotWorldTruth||
        !p.OwnerAndPOVVerified||len(p.LedgerDigests)!=5||
        len(p.LedgerCounts)!=5||b.BKnowledgeInputs!=1||b.BReceivedAt!=35||
        !b.POVIsolated||!b.RestoredAfterReopen||!c.ValidatorIndeterminate||
        !c.RetryAuthorizedAfterRestart{
        return zero,errors.New("C4 public reporting failed exact cross-slice proof guard")
    }
    d:=make(map[string]string,len(p.LedgerDigests))
    counts:=make(map[string]int,len(p.LedgerCounts))
    for _,name:=range referenceC4LedgerNames{
        digest,ok:=p.LedgerDigests[name]
        if !ok||len(digest)!=64||p.LedgerCounts[name]<=0{
            return zero,errors.New("C4 public integrity proof omitted or malformed canonical ledger")
        }
        d[name]=digest;counts[name]=p.LedgerCounts[name]
    }
    return ReferenceC4PublicReport{
        Status:r.Status,RunID:r.RunID,CommitCount:len(r.Cycles),
        Delivery:ReferenceC4Delivery{
            ClaimID:a.ClaimID,TransmissionID:a.TransmissionID,
            DeliveryTaskID:a.DeliveryTaskID,ReceivedAt:b.BReceivedAt},
        Owner:ReferenceC4Owner{SnapshotID:b.SnapshotID,AReceiptID:b.AOwnerReceiptID,
            BReceiptID:b.OwnerReceiptID,RestoredAfterReopen:b.RestoredAfterReopen,
            POVIsolated:b.POVIsolated},
        Retry:ReferenceC4Retry{AbortID:c.AbortID,RetryID:c.RetryID,
            ValidatorIndeterminate:c.ValidatorIndeterminate,
            AuthorizedAfterReopen:c.RetryAuthorizedAfterRestart},
        Integrity:ReferenceC4Integrity{LedgerDigests:d,LedgerCounts:counts,
            FiveLedgersVerified:p.FiveLedgersVerified,
            OutboxAndScheduleVerified:p.OutboxAndScheduleVerified,
            SnapshotAndReplayVerified:p.SnapshotAndReplayVerified,
            ClaimNotWorldTruth:p.ClaimNotWorldTruth},
    },nil
}
