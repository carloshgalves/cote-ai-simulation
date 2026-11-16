package commit

import (
    "bytes"
    "context"
    "crypto/ed25519"
    "encoding/hex"
    "errors"
    "fmt"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

// ReferenceAbortRetryProof records rooted public audit identities, never
// provisional decisions, untrusted actor payloads or private signing keys.
type ReferenceAbortRetryProof struct {
    AbortID string `json:"abort_id"`
    AbortDigest string `json:"abort_digest"`
    RetryID string `json:"retry_id"`
    RetryDigest string `json:"retry_digest"`
    PreviousFenceID string `json:"previous_fence_id"`
    RetryFenceID string `json:"retry_fence_id"`
    PreviousInputDigest string `json:"previous_input_digest"`
    RetryInputDigest string `json:"retry_input_digest"`
    OperatorKeyFingerprint string `json:"operator_key_fingerprint"`
    OperatorApprovalDigest string `json:"operator_approval_digest"`
    IndeterminateSubjects int `json:"indeterminate_subjects"`
    ValidatorIndeterminate bool `json:"validator_indeterminate"`
    HaltedAfterRestart bool `json:"halted_after_restart"`
    UnauthorizedFenceDenied bool `json:"unauthorized_fence_denied"`
    RetryAuthorizedAfterRestart bool `json:"retry_authorized_after_restart"`
    SameInputDigest bool `json:"same_input_digest"`
    SameMembership bool `json:"same_membership"`
    SamePolicyAndClock bool `json:"same_policy_and_clock"`
    AbortRevision uint64 `json:"abort_revision"`
    AbortEvents int `json:"abort_events"`
    AbortCommitCount int `json:"abort_commit_count"`
    RetryAttempt uint64 `json:"retry_attempt"`
    ReplayVerified bool `json:"replay_verified"`
}

// This is an explicitly enrolled *demo* operator in the command's trusted
// control boundary, not an actor-provided reason string. A frozen local
// Ed25519 key signs the exact run/cycle/abort/fence/decision digest before
// the caller may invoke the privileged Coordinator.Retry. It is deterministic
// only for reproducible synthetic fixtures and must never be used in prod.
// The normative SQLite AttemptRetryRecord remains the sole causal authority.
func referenceC3OperatorKey(run domain.ID) ed25519.PrivateKey {
    preimage:=append([]byte("cote.csf.reference.c3.operator.test-only.v1"),run[:]...)
    seed:=identity.OpaqueContentHash(preimage)
    return ed25519.NewKeyFromSeed(seed[:])
}
func referenceC3Authorize(ctx context.Context,b *policy.Bundle,run,cycle domain.ID,
    abort,priorFence [32]byte,priorDigest [32]byte,forge bool)([32]byte,[32]byte,error){
    var zero [32]byte
    if err:=ctx.Err();err!=nil{return zero,zero,err}
    if b==nil{return zero,zero,errors.New("C3 operator needs a pinned policy")}
    token,err:=codec.CanonicalPayload([]any{"cote.csf.reference.c3.operator-authorization.v1",
        run[:],cycle[:],abort[:],priorFence[:],priorDigest[:],"operator.retry",uint64(1)})
    if err!=nil{return zero,zero,err}
    signer:=referenceC3OperatorKey(run)
    defer clear(signer)
    public:=signer.Public().(ed25519.PublicKey)
    signature:=ed25519.Sign(signer,token)
    if forge{signature[0]^=0x80}
    // Enrolled in the local harness by exact test-only key/role binding,
    // not by the unverifiable sender/presenter of an ActionProposal.
    trusted:=referenceC3OperatorKey(run)
    defer clear(trusted)
    registered:=trusted.Public().(ed25519.PublicKey)
    if !bytes.Equal(public,registered)||!ed25519.Verify(registered,token,signature){
        return zero,zero,errors.New("C3 operator signature not authorized by registered local signer")
    }
    p:=identity.OpaqueContentHash(registered)
    approval:=identity.OpaqueContentHash(token)
    return p,approval,nil
}

// ReopenC3 is deliberately a distinct causal SQLite session after an actual
// HALTED_ON_ABORT terminal. It does not synthesize a retry fence, bypass the
// InputLedger, or choose new members; the original admitted source set and
// policy versions must survive the canonical AdmissionFence retry guard.
type referenceC3Session struct {
    Store *sqlite.Store
    Ledger *inputledger.Ledger
    Coordinator *Coordinator
    Cycle admission.Cycle
    Fence admission.Fence
    Proof *ReferenceAbortRetryProof
}

func referenceC3Same(a,b any) bool {
    x,err:=codec.CanonicalPayload(a)
    if err!=nil{return false}
    y,err:=codec.CanonicalPayload(b)
    return err==nil&&bytes.Equal(x,y)
}
func referenceC3RowCount(ctx context.Context,path,table string)(int,error){
    switch table {case "event_store","committed_cycles":default:return 0,errors.New("C3 queried unregistered table")}
    db,err:=openReferenceReadOnlyDB(path)
    if err!=nil{return 0,err}
    defer db.Close()
    var n int
    err=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM "+table).Scan(&n)
    return n,err
}

func referenceC3AbortRetry(ctx context.Context,path string,b *policy.Bundle,g *genesis.Genesis,
    store *sqlite.Store,ledger *inputledger.Ledger,c *Coordinator,policies admission.Policies,
    cycle admission.Cycle,fence admission.Fence,options ReferenceOptions)(referenceC3Session,error){
    var out referenceC3Session
    // No predeclared winner is permitted: run the actual deterministic
    // validator with the stock's accessibility facet marked unknown.
    // This simulates epistemic uncertainty, not a corrupted SQLite state.
    before,worldDigest,err:=store.LoadWorldState(ctx,uint64(cycle.BaseRevision))
    if err!=nil{return out,err}
    stock,err:=reducer.DecodeStock(before)
    if err!=nil||stock.Available!=1||worldDigest!=domain.Digest(cycle.BaseStateHash){
        return out,errors.New("C3 requires exact uncontested inventory state before terminal")
    }
    actors:=map[string]bool{"school:a":true,"school:b":true}
    forbidden:=map[string]bool{"school:a":false,"school:b":false}
    view,_,err:=ledger.AdmissionView(ctx,b)
    if err!=nil{return out,err}
    request,err:=decision.FromPersistedFence(ctx,b,view,cycle,policies,fence,store,
        decision.Snapshot{BaseRevision:cycle.BaseRevision,ResourceKind:"inventory",
            ResourceID:"last.resource",Available:1,Known:false,
            AccessibleActors:actors,ForbiddenActors:forbidden},
        []decision.ActionSchema{{Type:"resource.consume",Version:1,
            Operation:"resource.consume",Producer:"resource.consume"}},
        g.WorldSeed,nil)
    if err!=nil{return out,fmt.Errorf("C3 authentic validator input: %w",err)}
    evaluated,validationErr:=decision.Run(request)
    if !errors.Is(validationErr,decision.ErrIndeterminate)||
        len(evaluated.Indeterminate)!=2||len(evaluated.Dispositions)!=0 {
        return out,fmt.Errorf("C3 validator did not return 2 INDETERMINATE and 0 final dispositions: %w",validationErr)
    }
    c.failAfterStep=options.FailAbortAt
    abort,err:=c.Abort(ctx,g.RunID,cycle.ID,fence,0,evaluated.Indeterminate)
    c.failAfterStep=0
    if err!=nil{return out,fmt.Errorf("C3 atomic terminal abort: %w",err)}
    after,err:=store.LoadClock(ctx,[32]byte(g.RunID))
    if err!=nil{return out,err}
    if after.Revision!=cycle.BaseRevision||after.Instant!=cycle.Instant||
        after.LastOrdinal!=-1{return out,errors.New("C3 aborted cycle advanced world clock")}
    worldAfter,hashAfter,err:=store.LoadWorldState(ctx,uint64(after.Revision))
    if err!=nil||hashAfter!=worldDigest||!referenceC3Same(worldAfter,before){
        return out,errors.New("C3 aborted validator mutated world or stock")
    }
    events,err:=referenceC3RowCount(ctx,path,"event_store")
    if err!=nil{return out,err}
    commits,err:=referenceC3RowCount(ctx,path,"committed_cycles")
    if err!=nil{return out,err}
    if events!=0||commits!=0{return out,errors.New("C3 abort published provisional events or cycle commit")}
    if err:=store.Close();err!=nil{return out,err}
    resumed,err:=sqlite.Open(ctx,path)
    if err!=nil{return out,err}
    // Return the live store only after all validation, else close.
    finished:=false
    defer func(){if !finished{resumed.Close()}}()
    reopenedLedger,err:=inputledger.Open(ctx,g.RunID,b,resumed)
    if err!=nil{return out,err}
    ctl,err:=resumed.LoadCycleControl(ctx,[32]byte(g.RunID))
    if err!=nil||len(ctl)!=7||ctl[0]!=uint64(3)||ctl[6]!=uint64(1){
        return out,fmt.Errorf("C3 restart lost HALTED_ON_ABORT: %v",err)
    }
    replayBefore,err:=resumed.ReplayClock(ctx,[32]byte(g.RunID),b)
    if err!=nil||replayBefore.ReplayedCommits!=0{
        return out,fmt.Errorf("C3 replay changed aborted world history: %v",err)
    }
    frozen:=cycle
    frozen.Attempt=1
    frozen.RetryRef=[]any{uint64(0)}
    // A missing, guessed or forged RetryRef cannot lift HALTED_ON_ABORT.
    guard:=admission.NewCoordinator(admission.NewFenceLog(resumed))
    if _,err:=guard.AdmitThenEvaluate(ctx,b,reopenedLedger,frozen,policies,nil);err==nil{
        return out,errors.New("C3 admitted a second attempt without authoritative RetryRecord")
    }
    stopped,err:=resumed.LoadCycleControl(ctx,[32]byte(g.RunID))
    if err!=nil||stopped[0]!=uint64(3){
        return out,errors.New("C3 unauthorized attempt changed durable HALTED state")
    }
    signerFingerprint,approvalDigest,err:=referenceC3Authorize(ctx,b,g.RunID,cycle.ID,
        [32]byte(abort.Identity),[32]byte(fence.ID),[32]byte(fence.Digest),
        options.ForgeRetryOperator)
    if err!=nil{return out,err}
    operator:=New(resumed,b,policies)
    operator.failAfterStep=options.FailRetryAt
    retry,err:=operator.Retry(ctx,g.RunID,cycle.ID,abort,"operator.retry")
    operator.failAfterStep=0
    if err!=nil{return out,fmt.Errorf("C3 operator-authorized canonical retry: %w",err)}
    // The AttemptRetryRecord is the single authoritative state change,
    // not a loop that silently repeats SQL after a failed transaction.
    ctrl,err:=resumed.LoadCycleControl(ctx,[32]byte(g.RunID))
    if err!=nil||ctrl[0]!=uint64(2)||ctrl[6]!=uint64(1){
        return out,errors.New("C3 authorized attempt was not durably RETRY_AUTHORIZED")
    }
    if _,err:=operator.Retry(ctx,g.RunID,cycle.ID,abort,"operator.retry");err==nil{
        return out,errors.New("C3 second retry incorrectly accepted")
    }
    retryFields,err:=codec.StrictDecodePayload(retry.Canonical)
    if err!=nil{return out,err}
    digest,_,err:=identity.New(b).Sum("cote.csf.digest.attempt-retry",
        "cote.csf.schema.attempt-retry",1,retryFields)
    if err!=nil{return out,err}
    if err:=resumed.Close();err!=nil{return out,err}
    // A second genuine reopen proves the RetryRecord survives an independent
    // store connection before the *new* fence is opened.
    recovered,err:=sqlite.Open(ctx,path)
    if err!=nil{return out,err}
    completed:=false
    defer func(){if !completed{recovered.Close()}}()
    ledgerAgain,err:=inputledger.Open(ctx,g.RunID,b,recovered)
    if err!=nil{return out,err}
    accepted,err:=recovered.LoadCycleControl(ctx,[32]byte(g.RunID))
    if err!=nil||accepted[0]!=uint64(2)||accepted[6]!=uint64(1){
        return out,errors.New("C3 restart lost RETRY_AUTHORIZED")
    }
    frozen.RetryRef=[]any{uint64(1),[]any{uint64(14),retry.Identity[:],digest[:]}}
    nextFence,err:=admission.NewCoordinator(admission.NewFenceLog(recovered)).
        AdmitThenEvaluate(ctx,b,ledgerAgain,frozen,policies,nil)
    if err!=nil{return out,fmt.Errorf("C3 frozen source cohort rejected on retry: %w",err)}
    immutable:=[]int{0,1,4,5,6,7,8,9,10,11,12,13,14,15,16}
    for _,i:=range immutable{
        if !referenceC3Same(nextFence.Fields[i],fence.Fields[i]){
            return out,fmt.Errorf("C3 new fence altered frozen causal/policy/membership field %d",i)
        }
    }
    sameMembers:=referenceC3Same(nextFence.Fields[11],fence.Fields[11])
    samePolicies:=referenceC3Same(nextFence.Fields[16],fence.Fields[16])
    sameInputs:=nextFence.InputDigest==fence.InputDigest
    if !sameMembers||!samePolicies||!sameInputs||nextFence.ID==fence.ID||
        nextFence.Fields[2]!=uint64(1){
        return out,errors.New("C3 retry did not preserve exact inputs/policy/attempt")
    }
    proof:=&ReferenceAbortRetryProof{
        AbortID:hex.EncodeToString(abort.Identity[:]),
        AbortDigest:hex.EncodeToString(abort.Canonical[len(abort.Canonical)-32:]),
        RetryID:hex.EncodeToString(retry.Identity[:]),
        RetryDigest:hex.EncodeToString(digest[:]),
        PreviousFenceID:hex.EncodeToString(fence.ID[:]),
        RetryFenceID:hex.EncodeToString(nextFence.ID[:]),
        PreviousInputDigest:hex.EncodeToString(fence.InputDigest[:]),
        RetryInputDigest:hex.EncodeToString(nextFence.InputDigest[:]),
        OperatorKeyFingerprint:hex.EncodeToString(signerFingerprint[:]),
        OperatorApprovalDigest:hex.EncodeToString(approvalDigest[:]),
        IndeterminateSubjects:len(evaluated.Indeterminate),
        ValidatorIndeterminate:true,HaltedAfterRestart:true,UnauthorizedFenceDenied:true,
        RetryAuthorizedAfterRestart:true,SameInputDigest:sameInputs,SameMembership:sameMembers,
        SamePolicyAndClock:samePolicies,AbortRevision:uint64(after.Revision),
        AbortEvents:events,AbortCommitCount:commits,RetryAttempt:uint64(frozen.Attempt),
        ReplayVerified:true,
    }
    completed=true;finished=true
    return referenceC3Session{Store:recovered,Ledger:ledgerAgain,
        Coordinator:New(recovered,b,policies),Cycle:frozen,Fence:nextFence,Proof:proof},nil
}
