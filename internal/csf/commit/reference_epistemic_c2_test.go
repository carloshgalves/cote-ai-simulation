package commit

import (
    "bytes"
    "context"
    "encoding/hex"
    "path/filepath"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// RED: C1 has durable addressed evidence but no owner snapshot/restore or
// sealed actor capability proof in the SAME six-commit simulation.
func TestReferenceC2IntegratedOwnerAndPOV(t *testing.T) {
    ctx:=context.Background()
    dir:=t.TempDir()
    got,err:=RunReferenceC2(ctx,filepath.Join(dir,"causal.db"),filepath.Join(dir,"owner.db"),bundlePath,
        ReferenceOptions{})
    if err!=nil{t.Fatal(err)}
    p:=got.Epistemic
    if got.Status!="RUN_LOCAL_10C2"||p==nil||got.FinalRevision!=6||got.FinalStock!=0||len(got.Cycles)!=6{
        t.Fatalf("C2 did not preserve C1's real six-commit run: %+v",got)
    }
    if p.SnapshotID==""||p.OwnerReceiptID==""||p.OwnerCheckpointHash==""||
        !p.RestoredAfterReopen||!p.WorkDeniedBeforeACK||!p.POVIsolated||
        !p.BarrierVerified||p.BReceivedAt!=35||p.BKnowledgeInputs!=1||!p.CompletionIdempotent{
        t.Fatalf("C2 did not prove actor scope/restart/receipt/barrier: %+v",p)
    }
    if p.OwnerCheckpointHash==got.FinalWorldHash{
        t.Fatal("owner hash substituted for canonical world-state hash")
    }
    if got.Communication==nil||got.Communication.DeliveryTaskID==""{t.Fatal("C1 delivery evidence absent")}
}

func TestReferenceC2OwnerAndLedgerPermutation(t *testing.T){
    var prior ReferenceResult
    for i,opt:=range []ReferenceOptions{{},{ReverseSources:true},{ReverseResponses:true},
        {ReverseSources:true,ReverseResponses:true}}{
        dir:=t.TempDir()
        got,err:=RunReferenceC2(context.Background(),filepath.Join(dir,"causal.db"),
            filepath.Join(dir,"owner.db"),bundlePath,opt)
        if err!=nil{t.Fatal(i,err)}
        if i==0{prior=got;continue}
        if got.RunID!=prior.RunID||got.EventStreamSHA256!=prior.EventStreamSHA256||
            !bytes.Equal(got.EventBytes,prior.EventBytes)||
            got.Epistemic.SnapshotID!=prior.Epistemic.SnapshotID||
            got.Epistemic.OwnerReceiptID!=prior.Epistemic.OwnerReceiptID{
            t.Fatalf("source/worker completion order altered exact causal/owner bytes at %d",i)
        }
    }
}

func TestReferenceC2NoOwnerOrSharedDatabaseFailsBeforeRun(t *testing.T){
    dir:=t.TempDir()
    path:=filepath.Join(dir,"causal.db")
    if _,err:=RunReferenceC2(context.Background(),path,path,bundlePath,ReferenceOptions{});err==nil{
        t.Fatal("owner may not be same SQLite authority as causal simulation")
    }
    if _,err:=RunReferenceC2(context.Background(),path,"",bundlePath,ReferenceOptions{});err==nil{
        t.Fatal("owner path missing, but C2 run permitted")
    }
}

func TestReferenceC2OwnerCheckpointBytePin(t *testing.T){
    dir:=t.TempDir()
    got,err:=RunReferenceC2(context.Background(),filepath.Join(dir,"causal.db"),
        filepath.Join(dir,"owner.db"),bundlePath,ReferenceOptions{})
    if err!=nil{t.Fatal(err)}
    original,err:=hex.DecodeString(got.Epistemic.OwnerCheckpointHash)
    if err!=nil||len(original)!=32{t.Fatal("checkpoint content digest not returned")}
    value,err:=codec.CanonicalPayload([]any{"reference.c2.actor-b",got.Communication.DeliveryTaskID,
        got.Communication.KnowledgeInputID,int64(35),uint64(6)})
    if err!=nil{t.Fatal(err)}
    digest:=identity.OpaqueContentHash(value)
    if !bytes.Equal(original,digest[:]){t.Fatal("owner checkpoint not derived exclusively from addressed, causal B evidence")}
}
