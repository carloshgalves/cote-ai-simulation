package commit

import (
    "bytes"
    "context"
    "database/sql"
    "encoding/hex"
    "runtime"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "path/filepath"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    _ "modernc.org/sqlite"
)

// RED: the #116 tracer has no true-claim-neutral communication follow-on.
func TestReferenceCommunicationC1SameRunAndDueDelivery(t *testing.T) {
    ctx := context.Background()
    dbPath := filepath.Join(t.TempDir(), "c1.db")
    got, err := RunReference(ctx, dbPath, bundlePath, ReferenceOptions{IncludeCommunication: true})
    if err != nil { t.Fatal(err) }
    if got.Status != "RUN_LOCAL_10C1" || len(got.Cycles) != 6 || got.FinalRevision != 6 || got.FinalInstant != 35 || got.FinalStock != 0 {
        t.Fatalf("communication not part of original run: %+v", got)
    }
    if got.Communication == nil || got.Communication.ClaimID == "" || got.Communication.TransmissionID == "" ||
        got.Communication.SentAt != 10 || got.Communication.DeliveredAt != 35 ||
        got.Communication.BKnowledgeInputs != 1 || got.Communication.KnowledgeInputID == "" ||
        got.Communication.DeliveryTaskID == "" || !got.Communication.ReplayVerified {
        t.Fatalf("missing authoritative claim and addressed delivery proof: %+v", got.Communication)
    }
    db, err := sql.Open("sqlite", "file:"+dbPath)
    if err != nil { t.Fatal(err) }
    defer db.Close()
    for _, c := range []struct{ table string; want int }{
        {"genesis_records",1}, {"communication_claims",1}, {"communication_transmissions",1},
        {"committed_cycles",6}, {"world_revisions",7},
    } {
        var count int
        if err := db.QueryRowContext(ctx,"SELECT COUNT(*) FROM "+c.table).Scan(&count); err != nil || count != c.want {
            t.Fatalf("%s count %d, want %d: %v",c.table,count,c.want,err)
        }
    }
    var delivered int
    if err := db.QueryRowContext(ctx,"SELECT COUNT(*) FROM communication_transmissions WHERE status=1").Scan(&delivered); err != nil || delivered != 1 {
        t.Fatalf("delayed transmission not terminal exactly once: %d / %v",delivered,err)
    }
    rows, err := db.QueryContext(ctx,"SELECT canonical FROM event_store")
    if err != nil { t.Fatal(err) }
    defer rows.Close()
    sent, due := 0, 0
    for rows.Next() {
        var raw []byte
        if err := rows.Scan(&raw); err != nil { t.Fatal(err) }
        val, err := codec.StrictDecodePayload(raw)
        if err != nil { t.Fatal(err) }
        f := val.([]any)
        switch f[3] {
        case "resource.stolen": t.Fatal("false claim promoted to factual world event")
        case "communication.sent": sent++
        case "communication.delivered": due++
        }
    }
    if err := rows.Err(); err != nil { t.Fatal(err) }
    if sent != 1 || due != 1 { t.Fatalf("send/delivery Event count %d/%d",sent,due) }
}

func TestReferenceCommunicationC1SourceWorkerPermutationByteExact(t *testing.T) {
    var event, claimFence []byte
    var result string
    for n, v := range []ReferenceOptions{
        {IncludeCommunication:true},
        {IncludeCommunication:true,ReverseSources:true},
        {IncludeCommunication:true,ReverseResponses:true},
        {IncludeCommunication:true,ReverseSources:true,ReverseResponses:true},
    } {
        got, err := RunReference(context.Background(),filepath.Join(t.TempDir(),"run.db"),bundlePath,v)
        if err != nil { t.Fatal(n,err) }
        if n == 0 {
            event=bytes.Clone(got.EventBytes)
            claimFence=bytes.Clone(got.CycleFenceBytes[3])
            result=got.Communication.ClaimID
        } else if !bytes.Equal(event,got.EventBytes) || !bytes.Equal(claimFence,got.CycleFenceBytes[3]) ||
            result!=got.Communication.ClaimID { t.Fatalf("worker/source order changed causal bytes at permutation %d",n) }
    }
}

func TestReferenceCommunicationC1AtomicFailures(t *testing.T) {
    for _, step := range []uint8{1,2,3,4,5,6,7} {
        for _, failAt := range []string{"send","delivery"} {
            t.Run(failAt+"-"+string(rune('0'+step)),func(t *testing.T) {
                opt := ReferenceOptions{IncludeCommunication:true}
                beforeCommits := 3
                if failAt=="send" { opt.FailCommunicationSendAt=step } else { opt.FailCommunicationDeliveryAt=step; beforeCommits=5 }
                dbPath:=filepath.Join(t.TempDir(),"fail.db")
                if _,err:=RunReference(context.Background(),dbPath,bundlePath,opt);err==nil { t.Fatal("injected SQLite fault did not block publication") }
                db,err:=sql.Open("sqlite","file:"+dbPath)
                if err!=nil{t.Fatal(err)}
                defer db.Close()
                var commits int
                if err:=db.QueryRow("SELECT COUNT(*) FROM committed_cycles").Scan(&commits);err!=nil||commits!=beforeCommits{
                    t.Fatalf("partial communication commit %d, expected %d: %v",commits,beforeCommits,err)
                }
                var tx,claim int
                if err:=db.QueryRow("SELECT COUNT(*) FROM communication_claims").Scan(&claim);err!=nil{t.Fatal(err)}
                if err:=db.QueryRow("SELECT COUNT(*) FROM communication_transmissions").Scan(&tx);err!=nil{t.Fatal(err)}
                want:=0;if failAt=="delivery"{want=1}
                if claim!=want||tx!=want{t.Fatalf("partial claim/transmission publication %d/%d",claim,tx)}
                var delivered int
                if err:=db.QueryRow("SELECT COUNT(*) FROM communication_transmissions WHERE status=1").Scan(&delivered);err!=nil||delivered!=0{
                    t.Fatalf("partial delivery despite fault: %d %v",delivered,err)
                }
            })
        }
    }
}

func TestReferenceCommunicationC1ForgedAuthorRejected(t *testing.T){
    dbPath:=filepath.Join(t.TempDir(),"forged.db")
    _,err:=RunReference(context.Background(),dbPath,bundlePath,
        ReferenceOptions{IncludeCommunication:true,ForgeCommunicationActor:true})
    if err==nil{t.Fatal("forged sender was admitted as an authentic slot response")}
    db,e:=sql.Open("sqlite","file:"+dbPath);if e!=nil{t.Fatal(e)};defer db.Close()
    var count int
    if e:=db.QueryRow("SELECT COUNT(*) FROM communication_claims").Scan(&count);e!=nil||count!=0{
        t.Fatalf("unauthorized actor published a claim: %d %v",count,e)
    }
}

func TestReferenceCommunicationC1EncodedReadOnlyURIPath(t *testing.T){
    if runtime.GOOS=="windows"{t.Skip("Windows forbids '?' in file names")}
    path:=filepath.Join(t.TempDir(),"causal?mode=memory#forged.db")
    got,err:=RunReference(context.Background(),path,bundlePath,ReferenceOptions{IncludeCommunication:true})
    if err!=nil{t.Fatalf("read-only reference selectors confused URI metacharacters: %v",err)}
    if got.FinalRevision!=6||got.Communication==nil||!got.Communication.ReplayVerified{
        t.Fatalf("C1 did not use exact reserved SQLite path: %+v",got)
    }
}

func TestReferenceCommunicationC1TamperedDurableEvidenceFailsClosed(t *testing.T){
    cases:=[]struct{table string;byProof bool}{
        {"communication_claims",false},
        {"communication_transmissions",false},
        {"causal_outbox",true},
        {"evidence_ledger",true},
    }
    for _,tc:=range cases{t.Run(tc.table,func(t *testing.T){
        ctx:=context.Background()
        path:=filepath.Join(t.TempDir(),"tamper.db")
        result,err:=RunReference(ctx,path,bundlePath,ReferenceOptions{IncludeCommunication:true})
        if err!=nil{t.Fatal(err)}
        db,err:=sql.Open("sqlite","file:"+path)
        if err!=nil{t.Fatal(err)}
        var id,raw []byte
        if tc.byProof{
            name:=result.Communication.DeliveryTaskID
            if tc.table=="evidence_ledger"{name=result.Communication.KnowledgeInputID}
            id,err=hex.DecodeString(name)
            if err!=nil{db.Close();t.Fatal(err)}
            if err:=db.QueryRowContext(ctx,"SELECT canonical FROM "+tc.table+" WHERE id=?",id).Scan(&raw);err!=nil{db.Close();t.Fatal(err)}
        }else{
            if err:=db.QueryRowContext(ctx,"SELECT id,canonical FROM "+tc.table+" LIMIT 1").Scan(&id,&raw);err!=nil{db.Close();t.Fatal(err)}
        }
        if len(raw)<8{db.Close();t.Fatal("canonical communication root unexpectedly short")}
        damaged:=bytes.Clone(raw);damaged[len(damaged)-2]^=0x7f
        if _,err:=db.ExecContext(ctx,"UPDATE "+tc.table+" SET canonical=? WHERE id=?",damaged,id);err!=nil{db.Close();t.Fatal(err)}
        if err:=db.Close();err!=nil{t.Fatal(err)}
        reopened,err:=sqlite.Open(ctx,path)
        if err!=nil{return} // fail closed during adapter verification
        defer reopened.Close()
        bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
        run,err:=hex.DecodeString(result.RunID);if err!=nil{t.Fatal(err)}
        var runID [32]byte;copy(runID[:],run)
        if _,err:=reopened.ReplayClock(ctx,runID,bundle);err==nil{
            t.Fatal("causal replay trusted a tampered integrated C1 communication root")
        }
    })}
}

func TestReferenceCommunicationC1ForeignRecipientRouteFailsClosed(t *testing.T){
    ctx:=context.Background()
    path:=filepath.Join(t.TempDir(),"reroute.db")
    result,err:=RunReference(ctx,path,bundlePath,ReferenceOptions{IncludeCommunication:true})
    if err!=nil{t.Fatal(err)}
    task,err:=hex.DecodeString(result.Communication.DeliveryTaskID)
    if err!=nil{t.Fatal(err)}
    db,err:=sql.Open("sqlite","file:"+path)
    if err!=nil{t.Fatal(err)}
    changes,err:=db.ExecContext(ctx,"UPDATE evidence_routes SET holder_id='a' WHERE task_id=? AND kind=1",task)
    if err!=nil{db.Close();t.Fatal(err)}
    changed,err:=changes.RowsAffected();if err!=nil||changed!=1{db.Close();t.Fatal("missing B input route",changed,err)}
    if err:=db.Close();err!=nil{t.Fatal(err)}
    store,err:=sqlite.Open(ctx,path)
    if err!=nil{return}
    defer store.Close()
    b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
    rawRun,err:=hex.DecodeString(result.RunID);if err!=nil{t.Fatal(err)}
    var run [32]byte;copy(run[:],rawRun)
    if err:=store.CheckPerceptionBarrier(ctx,run,result.FinalRevision);err==nil{
        t.Fatal("barrier accepted forged actor A access to B-only delivered Claim")
    }
    if _,err:=store.ReplayClock(ctx,run,b);err==nil{
        t.Fatal("replay accepted swapped holder for B-delivered claim")
    }
}
