package commit

import (
    "bytes"
    "context"
    "database/sql"
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
        got.Communication.BKnowledgeInputs != 1 || !got.Communication.ReplayVerified {
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
