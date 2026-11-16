package commit

import (
    "bytes"
    "context"
    "crypto/ed25519"
    "encoding/hex"
    "errors"
    "fmt"
    "os"
    "path/filepath"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/contextboundary"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// ReferenceC2Proof intentionally contains NO cognitive private content,
// unredacted Claim, actual sender, engine-only WorldState or signing key.
// IDs and digest names below refer to already authenticated persisted records.
type ReferenceC2Proof struct {
    SnapshotID          string `json:"snapshot_id"`
    OwnerReceiptID      string `json:"owner_receipt_id"`
    OwnerCheckpointHash string `json:"owner_checkpoint_hash"`
    BReceivedAt         int64  `json:"b_received_at"`
    BKnowledgeInputs    int    `json:"b_knowledge_inputs"`
    BarrierVerified     bool   `json:"barrier_verified"`
    POVIsolated         bool   `json:"pov_isolated"`
    CompletionIdempotent bool  `json:"completion_idempotent"`
    RestoredAfterReopen bool   `json:"restored_after_reopen"`
    WorkDeniedBeforeACK bool   `json:"work_denied_before_ack"`
    AOwnerReceiptID      string `json:"a_owner_receipt_id"`
    AOwnerCheckpointHash string `json:"a_owner_checkpoint_hash"`
    SeparateOwnerKeys    bool   `json:"separate_owner_keys"`
}

// referenceC2SyntheticSigner is deliberately deterministic so that the local
// reproducibility experiment produces byte-identical signed owner snapshots.
// This is a SYNTHETIC TEST KEY, not a production trust root. It must not be
// used to sign real, remotely durable memory or pass external release gates.
func referenceC2SyntheticSigner(run [32]byte, actor string) ed25519.PrivateKey {
    preimage:=append([]byte("cote.csf.reference.c2.synthetic-ed25519-owner.v1." + actor),run[:]...)
    seed:=identity.OpaqueContentHash(preimage)
    return ed25519.NewKeyFromSeed(seed[:])
}

// referenceC2NextDispatch opens an actual next-cycle actor capability through
// InputLedger. It is not an out-of-band permission to read EvidenceLedger.
func referenceC2NextDispatch(ctx context.Context,store *sqlite.Store,ledger *inputledger.Ledger,
    bundle *policy.Bundle,run domain.ID,actor string,roundOrdinal uint32)(domain.ID,error) {
    g,err:=store.ValidatedGenesis(ctx,[32]byte(run),bundle)
    if err!=nil{return domain.ID{},err}
    state,err:=store.LoadClock(ctx,[32]byte(run))
    if err!=nil{return domain.ID{},err}
    if uint64(state.Revision)!=6||state.LastOrdinal!=0{
        return domain.ID{},errors.New("C2 future actor dispatch requires exact C1 completed clock")
    }
    ordinal:=domain.CycleOrdinal(state.LastOrdinal+1)
    eligibility:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant,NotBeforeCycleOrdinal:ordinal}
    origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
    round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school",actor},
        []string{"act"},eligibility,origin,0,roundOrdinal,0,[]any{uint64(0)})
    if err!=nil{return domain.ID{},err}
    cycle,err:=referenceC1Cycle(ctx,store,bundle,run,state,ordinal)
    if err!=nil{return domain.ID{},err}
    raw,_,err:=identity.New(bundle).Sum("cote.csf.id.dispatch",
        "cote.csf.schema.dispatch-id-preimage",1,
        []any{run[:],cycle.ID[:],round.ID[:],"act",uint64(0)})
    if err!=nil{return domain.ID{},err}
    dispatch:=inputledger.Dispatch{ID:domain.ID(raw),RunID:run,CycleID:cycle.ID,
        RoundID:round.ID,SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,
        BaseRevision:state.Revision,EffectiveAt:state.Instant}
    err=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
        ID:cycle.ID,BaseRevision:state.Revision,Instant:state.Instant})
    if err!=nil{return domain.ID{},err}
    return domain.ID(raw),nil
}

// referenceC2AddressedView is a strict holder-only cognitive context.
// It cannot inject secrets, privileged future canon or a shared global
// EventStore into agent A, agent B or a public context.
func referenceC2AddressedView(ctx context.Context,store *sqlite.Store,id domain.ID,
    actor [2]string)(contextboundary.View,error){
    inbox,err:=store.BindActorInbox(ctx,[32]byte(id),actor)
    if err!=nil{return contextboundary.View{},err}
    return contextboundary.Build(ctx,inbox,actor)
}

// RunReferenceC2 extends the *same* six-commit #126 causal run with true
// B-only evidenced context, signed owner escrow, a five-ledger snapshot,
// independently reopened SQLite owner/CSF, restore+durable ACK and cognitive
// callback gate. There is NO new causal writer or fake LLM actor.
func RunReferenceC2(ctx context.Context,dbPath,ownerPath,bundlePath string,
    options ReferenceOptions)(ReferenceResult,error){
    var zero ReferenceResult
    if ctx==nil||dbPath==""||ownerPath==""{return zero,errors.New("C2 requires explicit causal/owner paths and context")}
    absDB,err:=filepath.Abs(dbPath);if err!=nil{return zero,err}
    absOwner,err:=filepath.Abs(ownerPath);if err!=nil{return zero,err}
    absAOwner:=absOwner+".actor-a"
    if absDB==absOwner||absDB==absAOwner||absOwner==absAOwner{
        return zero,errors.New("cognitive actor owners must be distinct from causal SQLite and each other")
    }
    dir,err:=os.Stat(filepath.Dir(absOwner))
    if err!=nil{return zero,err}
    if !dir.IsDir()||dir.Mode().Perm()&0077!=0{
        return zero,errors.New("C2 owner store requires a private 0700 directory")
    }
    // Resolve pre-existing symlink path aliases before any causal work.
    for _,ownerFile:=range []string{absOwner,absAOwner}{
        if _,err:=os.Lstat(ownerFile);err==nil{
            return zero,errors.New("C2 requires fresh independent actor-owner databases")
        }else if !os.IsNotExist(err){return zero,err}
    }
    if _,err:=os.Lstat(absDB);err==nil{
        return zero,errors.New("C2 requires fresh causal database")
    }else if !os.IsNotExist(err){return zero,err}
    options.IncludeCommunication=true
    result,err:=RunReference(ctx,absDB,bundlePath,options)
    if err!=nil{return zero,err}
    if result.Status!="RUN_LOCAL_10C1"||result.Communication==nil||
        result.FinalRevision!=6||result.FinalInstant!=35{
        return zero,errors.New("C2 cannot begin without the exact six-commit C1 run")
    }
    bundle,err:=policy.Load(bundlePath);if err!=nil{return zero,err}
    rawRun,err:=hex.DecodeString(result.RunID)
    if err!=nil||len(rawRun)!=32{return zero,errors.New("C2 run identifier invalid")}
    var run domain.ID;copy(run[:],rawRun)
    ownerSigner:=referenceC2SyntheticSigner([32]byte(run),"b")
    defer clear(ownerSigner)
    ownerASigner:=referenceC2SyntheticSigner([32]byte(run),"a")
    defer clear(ownerASigner)
    store,err:=sqlite.Open(ctx,absDB)
    if err!=nil{return zero,err}
    defer store.Close()
    if err:=store.CheckPerceptionBarrier(ctx,[32]byte(run),6);err!=nil{
        return zero,fmt.Errorf("C2 pre-dispatch epistemic barrier: %w",err)
    }
    // Snapshot eligibility also includes every holder of addressed evidence.
    // The C1 resource rounds made A and B both eligible. A/B have distinct
    // real owner files and distinct Ed25519 signing keys: neither actor's
    // memory can be silently substituted for the other's.
    scope:=ownercheckpoint.Scope{Run:[32]byte(run),Namespace:"school",Actor:"b"}
    scopeA:=ownercheckpoint.Scope{Run:[32]byte(run),Namespace:"school",Actor:"a"}
    for _,registration:=range []struct{scope ownercheckpoint.Scope;signer ed25519.PrivateKey}{
        {scopeA,ownerASigner},{scope,ownerSigner},
    }{
        who:=registration.scope
        if err:=store.RegisterCognitiveActor(ctx,who.Run,who.Namespace,who.Actor,bundle);err!=nil{return zero,err}
        public:=registration.signer.Public().(ed25519.PublicKey)
        if err:=store.RegisterCognitiveOwnerKeyV1(ctx,who.Run,who.Namespace,who.Actor,public,bundle);err!=nil{return zero,err}
    }
    ledger,err:=inputledger.Open(ctx,run,bundle,store)
    if err!=nil{return zero,err}
    bDispatch,err:=referenceC2NextDispatch(ctx,store,ledger,bundle,run,"b",3)
    if err!=nil{return zero,fmt.Errorf("C2 B dispatch: %w",err)}
    aDispatch,err:=referenceC2NextDispatch(ctx,store,ledger,bundle,run,"a",4)
    if err!=nil{return zero,fmt.Errorf("C2 A dispatch: %w",err)}
    bView,err:=referenceC2AddressedView(ctx,store,bDispatch,[2]string{"school","b"})
    if err!=nil{return zero,fmt.Errorf("C2 recipient B context: %w",err)}
    aView,err:=referenceC2AddressedView(ctx,store,aDispatch,[2]string{"school","a"})
    if err!=nil{return zero,fmt.Errorf("C2 sender A context: %w",err)}
    inputRaw,err:=hex.DecodeString(result.Communication.KnowledgeInputID)
    if err!=nil||len(inputRaw)!=32{return zero,errors.New("C1 B evidence reference malformed")}
    var messageID [32]byte;copy(messageID[:],inputRaw)
    count,received:=0,int64(-1)
    for _,item:=range bView.Inbox{
        if item.Input.Identity==messageID{count++;received=item.ReceivedAt}
    }
    if count!=1||received!=35{return zero,errors.New("B's delivered Claim is not a unique causal KnowledgeInput at t=35")}
    for _,item:=range aView.Inbox{
        if item.Input.Identity==messageID{
            return zero,errors.New("B private Claim leaked into sender A context")
        }
    }
    bReader,err:=store.BindActorInbox(ctx,[32]byte(bDispatch),[2]string{"school","b"})
    if err!=nil{return zero,err}
    if _,err:=contextboundary.Build(ctx,bReader,[2]string{"school","a"});err==nil{
        return zero,errors.New("actor A obtained B holder-only context")
    }
    if _,err:=contextboundary.Build(ctx,nil,[2]string{"school","public"});err==nil{
        return zero,errors.New("public actor obtained owner-only epistemic context")
    }
    // This opaque synthetic owner memory binds only B's authenticated evidence
    // identities and causal received_at. The actual Claim payload is NEVER
    // written into this model-agnostic checkpoint.
    content,err:=codec.CanonicalPayload([]any{"reference.c2.actor-b",
        result.Communication.DeliveryTaskID,result.Communication.KnowledgeInputID,
        int64(35),uint64(6)})
    if err!=nil{return zero,err}
    seal:=identity.OpaqueContentHash(content)
    db,err:=openReferenceReadOnlyDB(absDB);if err!=nil{return zero,err}
    var evidenceCursor uint64
    if err:=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM evidence_ledger").Scan(&evidenceCursor);err!=nil{
        db.Close();return zero,err
    }
    if err:=db.Close();err!=nil{return zero,err}
    cp:=sqlite.SnapshotCheckpoint{ActorNamespace:scope.Namespace,ActorID:scope.Actor,
        StoreID:"reference-c2-owner-b",SchemaVersion:1,Content:bytes.Clone(content),
        Producer:"reference-c2-scripted",ProducerVersion:1,CoversInstant:35,
        CoversRevision:6,CoversEvidenceCursor:evidenceCursor}
    defer clear(cp.Content)
    // The sender A's opaque, synthetic private state is NOT a percept for B.
    // A's checkpoint is staged to an entirely separate local owner database,
    // signed by a distinct key. Only a single sealed CSF snapshot joins the
    // two private owner receipts at the same immutable causal cutoff.
    aEvidenceRefs:=make([]any,0,len(aView.Inbox))
    for _,item:=range aView.Inbox{aEvidenceRefs=append(aEvidenceRefs,item.Input.Identity[:])}
    contentA,err:=codec.CanonicalPayload([]any{
        "reference.c2.actor-a",aEvidenceRefs,"synthetic-a-private-only-memento",
    })
    if err!=nil{return zero,err}
    sealA:=identity.OpaqueContentHash(contentA)
    cpA:=sqlite.SnapshotCheckpoint{ActorNamespace:scopeA.Namespace,ActorID:scopeA.Actor,
        StoreID:"reference-c2-owner-a",SchemaVersion:1,Content:bytes.Clone(contentA),
        Producer:"reference-c2-scripted",ProducerVersion:1,CoversInstant:35,
        CoversRevision:6,CoversEvidenceCursor:evidenceCursor}
    defer clear(cpA.Content)
    for _,entry:=range bView.Inbox {
        if bytes.Contains(entry.Observation.Canonical,[]byte("synthetic-a-private-only-memento")) ||
           bytes.Contains(entry.Input.Canonical,[]byte("synthetic-a-private-only-memento")){
            return zero,errors.New("C2 A-only private memento leaked to B's addressed context")
        }
    }
    owner,err:=localowner.Open(ctx,absOwner,ownerSigner)
    if err!=nil{return zero,err}
    defer owner.Close()
    ownerA,err:=localowner.Open(ctx,absAOwner,ownerASigner)
    if err!=nil{return zero,err}
    defer ownerA.Close()
    gateA,err:=ownercheckpoint.NewGate(ownerA,store,bundle)
    if err!=nil{return zero,err}
    receiptA,err:=gateA.Stage(ctx,scopeA,cpA)
    if err!=nil{return zero,fmt.Errorf("C2 A private durable owner stage: %w",err)}
    gateB,err:=ownercheckpoint.NewGate(owner,store,bundle)
    if err!=nil{return zero,err}
    receiptB,err:=gateB.Stage(ctx,scope,cp)
    if err!=nil{return zero,fmt.Errorf("C2 B private durable owner stage: %w",err)}
    publishedSnapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{cpA,cp})
    if err!=nil{return zero,fmt.Errorf("C2 joint A/B sealed causal snapshot: %w",err)}
    ref:=localdemo.Reference{Snapshot:[32]byte(publishedSnapshot.ID),Receipt:receiptB}
    refA:=localdemo.Reference{Snapshot:[32]byte(publishedSnapshot.ID),Receipt:receiptA}
    if ref.Snapshot==([32]byte{})||ref.Receipt==([32]byte{})||refA.Receipt==([32]byte{})||
        ref.Receipt==refA.Receipt{
        return zero,errors.New("C2 two-owner snapshot/receipts missing or not independent")
    }
    h,err:=localdemo.New(owner,store,bundle,scope,func(context.Context)error{
        return errors.New("C2 cognitive callback must not run before restore")
    })
    if err!=nil{return zero,err}
    if err:=h.Run(ctx,ref);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged){
        return zero,errors.New("C2 ran cognitive work without acknowledged restore")
    }
    // Prove independent connections/session, not just reusing the same open
    // Gate permit in the process that published the checkpoint.
    if err:=owner.Close();err!=nil{return zero,err}
    if err:=ownerA.Close();err!=nil{return zero,err}
    if err:=store.Close();err!=nil{return zero,err}
    restoredCSF,err:=sqlite.Open(ctx,absDB)
    if err!=nil{return zero,err}
    defer restoredCSF.Close()
    restoredOwner,err:=localowner.Open(ctx,absOwner,ownerSigner)
    if err!=nil{return zero,err}
    defer restoredOwner.Close()
    restoredOwnerA,err:=localowner.Open(ctx,absAOwner,ownerASigner)
    if err!=nil{return zero,err}
    defer restoredOwnerA.Close()
    aWorkCalls:=0
    freshA,err:=localdemo.New(restoredOwnerA,restoredCSF,bundle,scopeA,func(ctx context.Context)error{
        view,err:=referenceC2AddressedView(ctx,restoredCSF,aDispatch,[2]string{"school","a"})
        if err!=nil{return err}
        for _,entry:=range view.Inbox{
            if entry.Input.Identity==messageID{
                return errors.New("C2 A owner accessed B-only communication input")
            }
        }
        aWorkCalls++
        return nil
    })
    if err!=nil{return zero,err}
    if err:=freshA.Run(ctx,refA);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged){
        return zero,errors.New("C2 A owner got WORK without ACK")
    }
    workCalls:=0
    fresh,err:=localdemo.New(restoredOwner,restoredCSF,bundle,scope,func(ctx context.Context)error{
        proof,err:=referenceC2AddressedView(ctx,restoredCSF,bDispatch,[2]string{"school","b"})
        if err!=nil{return err}
        matches:=0
        for _,entry:=range proof.Inbox{
            if entry.Input.Identity==messageID && entry.ReceivedAt==35{matches++}
        }
        if matches!=1{return errors.New("C2 restored owner cannot read one causally delivered B claim")}
        workCalls++
        return nil
    })
    if err!=nil{return zero,err}
    if err:=fresh.Run(ctx,ref);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged)||workCalls!=0{
        return zero,errors.New("C2 fresh process inherited unacknowledged cognitive permission")
    }
    forged:=ref
    forged.Receipt[0]^=0x81
    if err:=fresh.Restore(ctx,forged);err==nil{
        return zero,errors.New("C2 accepted forged signed owner receipt")
    }
    if err:=fresh.Run(ctx,ref);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged){
        return zero,errors.New("forged receipt left old owner permission active")
    }
    changed:=ref
    changed.Snapshot[0]^=0x42
    if err:=fresh.Restore(ctx,changed);err==nil{
        return zero,errors.New("C2 restored missing/foreign snapshot")
    }
    wrongActor:=scope
    wrongActor.Actor="a"
    alien,err:=localdemo.New(restoredOwner,restoredCSF,bundle,wrongActor,func(context.Context)error{
        return errors.New("foreign actor illegally reached work callback")
    })
    if err!=nil{return zero,err}
    if err:=alien.Restore(ctx,ref);err==nil{return zero,errors.New("actor A restored B checkpoint")}
    if err:=freshA.Restore(ctx,ref);err==nil{
        return zero,errors.New("A owner accepted B-signed checkpoint receipt")
    }
    if err:=fresh.Restore(ctx,refA);err==nil{
        return zero,errors.New("B owner accepted A-signed checkpoint receipt")
    }
    wrongRun:=scope
    wrongRun.Run[0]^=0x20
    foreign,err:=localdemo.New(restoredOwner,restoredCSF,bundle,wrongRun,func(context.Context)error{
        return errors.New("foreign run illegally reached callback")
    })
    if err!=nil{return zero,err}
    if err:=foreign.Restore(ctx,ref);err==nil{return zero,errors.New("foreign run restored B checkpoint")}
    if err:=fresh.Restore(ctx,ref);err!=nil{
        return zero,fmt.Errorf("C2 genuine restored owner ACK: %w",err)
    }
    if err:=fresh.Run(ctx,ref);err!=nil||workCalls!=1{
        return zero,fmt.Errorf("C2 restored recipient did not execute authorized callback once: %w",err)
    }
    if err:=freshA.Restore(ctx,refA);err!=nil{
        return zero,fmt.Errorf("C2 A private restore/ACK: %w",err)
    }
    if err:=freshA.Run(ctx,refA);err!=nil||aWorkCalls!=1{
        return zero,fmt.Errorf("C2 isolated A owner did not execute after its own ACK: %w",err)
    }
    taskRaw,err:=hex.DecodeString(result.Communication.DeliveryTaskID)
    if err!=nil||len(taskRaw)!=32{return zero,errors.New("C2 delivery task malformed")}
    var task [32]byte;copy(task[:],taskRaw)
    // A duplicate completion is a pure idempotent retry: no second B
    // KnowledgeInput, no new revision, and a byte-identical snapshot.
    if _,err:=restoredCSF.DeliverCommunicationPerception(ctx,task);err!=nil{return zero,err}
    countAfter,err:=referenceC1BInputs(ctx,absDB);if err!=nil{return zero,err}
    if countAfter!=referenceC2CountBInbox(bView){
        return zero,errors.New("C2 duplicate completion changed B's durable KnowledgeInput count")
    }
    if err:=restoredCSF.CheckPerceptionBarrier(ctx,[32]byte(run),6);err!=nil{return zero,err}
    snapshot,err:=restoredCSF.VerifySnapshotV1(ctx,[32]byte(run),ref.Snapshot,bundle)
    if err!=nil||snapshot.Snapshot.ID!=ref.Snapshot||snapshot.TailCommits!=0{
        return zero,fmt.Errorf("C2 exact immutable sealed snapshot not verified: %w",err)
    }
    if _,err:=restoredCSF.ReplayClock(ctx,[32]byte(run),bundle);err!=nil{return zero,err}
    result.Status="RUN_LOCAL_10C2"
    result.Epistemic=&ReferenceC2Proof{
        SnapshotID:hex.EncodeToString(ref.Snapshot[:]),
        OwnerReceiptID:hex.EncodeToString(ref.Receipt[:]),
        OwnerCheckpointHash:hex.EncodeToString(seal[:]),
        BReceivedAt:received,BKnowledgeInputs:count,
        BarrierVerified:true,POVIsolated:true,
        CompletionIdempotent:true,RestoredAfterReopen:true,
        WorkDeniedBeforeACK:true,
        AOwnerReceiptID:hex.EncodeToString(refA.Receipt[:]),
        AOwnerCheckpointHash:hex.EncodeToString(sealA[:]),
        SeparateOwnerKeys:true,
    }
    return result,nil
}

func referenceC2CountBInbox(view contextboundary.View)int{return len(view.Inbox)}
