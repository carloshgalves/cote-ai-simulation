package commit

import (
    "context"
    "crypto/ed25519"
    "database/sql"
    "encoding/hex"
    "errors"
    "os"
    "path/filepath"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    _ "modernc.org/sqlite"
)

func referenceC2AdversarialFixture(t *testing.T)(ReferenceResult,string,string,*policy.Bundle){
    t.Helper()
    dir:=t.TempDir()
    ownerDir:=filepath.Join(dir,"private-owner")
    if err:=os.Mkdir(ownerDir,0700);err!=nil{t.Fatal(err)}
    causal:=filepath.Join(dir,"causal.db")
    owner:=filepath.Join(ownerDir,"owner.db")
    got,err:=RunReferenceC2(context.Background(),causal,owner,bundlePath,ReferenceOptions{})
    if err!=nil{t.Fatal("real C2 run:",err)}
    bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
    return got,causal,owner,bundle
}

func referenceC2Decode32(t *testing.T,id string)[32]byte{
    t.Helper()
    raw,err:=hex.DecodeString(id)
    if err!=nil||len(raw)!=32{t.Fatal("invalid causal ID")}
    var out [32]byte;copy(out[:],raw);return out
}

func TestReferenceC2CorruptedPrivateOwnerFailsClosed(t *testing.T){
    got,causal,ownerPath,bundle:=referenceC2AdversarialFixture(t)
    ctx:=context.Background()
    db,err:=sql.Open("sqlite","file:"+ownerPath)
    if err!=nil{t.Fatal(err)}
    result,err:=db.ExecContext(ctx,"UPDATE checkpoints SET content=? WHERE actor_id='b'",[]byte("forged-owner-content"))
    if err!=nil{t.Fatal(err)}
    changed,err:=result.RowsAffected()
    if err!=nil||changed!=1{t.Fatal("B owner checkpoint not mutated",changed,err)}
    if err:=db.Close();err!=nil{t.Fatal(err)}
    run:=referenceC2Decode32(t,got.RunID)
    signer:=referenceC2SyntheticSigner(run,"b")
    defer clear(signer)
    store,err:=sqlite.Open(ctx,causal);if err!=nil{t.Fatal(err)}
    defer store.Close()
    owner,err:=localowner.Open(ctx,ownerPath,signer);if err!=nil{t.Fatal(err)}
    defer owner.Close()
    scope:=ownercheckpoint.Scope{Run:run,Namespace:"school",Actor:"b"}
    called:=false
    host,err:=localdemo.New(owner,store,bundle,scope,func(context.Context)error{called=true;return nil})
    if err!=nil{t.Fatal(err)}
    ref:=localdemo.Reference{
        Snapshot:referenceC2Decode32(t,got.Epistemic.SnapshotID),
        Receipt:referenceC2Decode32(t,got.Epistemic.OwnerReceiptID),
    }
    if err:=host.Restore(ctx,ref);err==nil{
        t.Fatal("signed escrow accepted corrupted external owner bytes")
    }
    if err:=host.Run(ctx,ref);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged)||called{
        t.Fatalf("corrupt private owner authorized cognitive work: %v",err)
    }
}

func TestReferenceC2WrongSignerCannotRestoreOtherActor(t *testing.T){
    got,causal,ownerPath,bundle:=referenceC2AdversarialFixture(t)
    ctx:=context.Background()
    run:=referenceC2Decode32(t,got.RunID)
    forgedSigner:=referenceC2SyntheticSigner(run,"a")
    defer clear(forgedSigner)
    store,err:=sqlite.Open(ctx,causal);if err!=nil{t.Fatal(err)}
    defer store.Close()
    wrongKeyOwner,err:=localowner.Open(ctx,ownerPath,forgedSigner)
    if err!=nil{t.Fatal(err)}
    defer wrongKeyOwner.Close()
    scope:=ownercheckpoint.Scope{Run:run,Namespace:"school",Actor:"b"}
    callback:=0
    host,err:=localdemo.New(wrongKeyOwner,store,bundle,scope,func(context.Context)error{callback++;return nil})
    if err!=nil{t.Fatal(err)}
    ref:=localdemo.Reference{Snapshot:referenceC2Decode32(t,got.Epistemic.SnapshotID),
        Receipt:referenceC2Decode32(t,got.Epistemic.OwnerReceiptID)}
    if err:=host.Restore(ctx,ref);err==nil{t.Fatal("A signer impersonated B owner")}
    if err:=host.Run(ctx,ref);err==nil||callback!=0{
        t.Fatal("wrong signer authorized B model callback")
    }
}

func TestReferenceC2MissingObservationRouteHoldsBarrierAndDispatch(t *testing.T){
    got,causal,_,bundle:=referenceC2AdversarialFixture(t)
    ctx:=context.Background()
    run:=referenceC2Decode32(t,got.RunID)
    task:=referenceC2Decode32(t,got.Communication.DeliveryTaskID)
    db,err:=sql.Open("sqlite","file:"+causal)
    if err!=nil{t.Fatal(err)}
    result,err:=db.ExecContext(ctx,
        "DELETE FROM evidence_routes WHERE task_id=? AND kind=0 AND holder_namespace='school' AND holder_id='b'",
        task[:])
    if err!=nil{db.Close();t.Fatal(err)}
    n,err:=result.RowsAffected()
    if err!=nil||n!=1{db.Close();t.Fatal("observation route did not exist",n,err)}
    if err:=db.Close();err!=nil{t.Fatal(err)}
    store,err:=sqlite.Open(ctx,causal);if err!=nil{t.Fatal(err)}
    defer store.Close()
    if err:=store.CheckPerceptionBarrier(ctx,run,6);err==nil{
        t.Fatal("missing B observation route released next-round barrier")
    }
    if _,err:=store.ReplayClock(ctx,run,bundle);err==nil{
        t.Fatal("replay accepted unpaired B KnowledgeInput")
    }
    if _,err:=store.VerifySnapshotV1(ctx,run,referenceC2Decode32(t,got.Epistemic.SnapshotID),bundle);err==nil{
        t.Fatal("snapshot verification accepted lost addressed observation")
    }
    ledger,err:=inputledger.Open(ctx,domain.ID(run),bundle,store)
    if err==nil{
        if _,err:=referenceC2NextDispatch(ctx,store,ledger,bundle,domain.ID(run),"b",12);err==nil{
            t.Fatal("future B dispatch bypassed incomplete epistemic barrier")
        }
    }
}

func TestReferenceC2MissingCompletionBlocksReopen(t *testing.T){
    got,causal,_,bundle:=referenceC2AdversarialFixture(t)
    ctx:=context.Background()
    run:=referenceC2Decode32(t,got.RunID)
    task:=referenceC2Decode32(t,got.Communication.DeliveryTaskID)
    db,err:=sql.Open("sqlite","file:"+causal);if err!=nil{t.Fatal(err)}
    rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
    if err!=nil{t.Fatal(err)}
    var completionID []byte
    for rows.Next(){
        var id,raw []byte
        if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();t.Fatal(err)}
        decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();t.Fatal(err)}
        fields,ok:=decoded.([]any)
        if !ok{rows.Close();t.Fatal("invalid C2 outbox record")}
        if len(fields)==5 {
            parent,ok:=fields[1].([]byte)
            if ok&&len(parent)==32&&string(parent)==string(task[:]){
                completionID=append([]byte(nil),id...)
            }
        }
    }
    if err:=rows.Err();err!=nil{rows.Close();t.Fatal(err)}
    if err:=rows.Close();err!=nil{t.Fatal(err)}
    if len(completionID)!=32{t.Fatal("B completion not present in true causal outbox")}
    _,err=db.ExecContext(ctx,"DELETE FROM causal_outbox WHERE id=?",completionID)
    if err!=nil{db.Close();t.Fatal(err)}
    if err:=db.Close();err!=nil{t.Fatal(err)}
    reopened,err:=sqlite.Open(ctx,causal);if err!=nil{t.Fatal(err)}
    defer reopened.Close()
    if err:=reopened.CheckPerceptionBarrier(ctx,run,6);err==nil{
        t.Fatal("missing durable completion released next dispatch")
    }
    if _,err:=reopened.ReplayClock(ctx,run,bundle);err==nil{
        t.Fatal("replay accepted absent B receipt")
    }
}

// The owner source accepts only Ed25519 keys for the explicitly enrolled actor,
// never the C1 message presenter or a caller-supplied observer's public claim.
var _ ed25519.PrivateKey = ed25519.PrivateKey{}
