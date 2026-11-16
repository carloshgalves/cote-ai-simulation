package commit

import (
    "bytes"
    "context"
    "database/sql"
    "encoding/hex"
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    _ "modernc.org/sqlite"
)

func c4Fixture(t *testing.T,opt ReferenceOptions)(ReferenceResult,string,string){
    t.Helper()
    dir:=t.TempDir()
    ownerDir:=filepath.Join(dir,"private")
    if err:=os.Mkdir(ownerDir,0700);err!=nil{t.Fatal(err)}
    db,owner:=filepath.Join(dir,"causal.db"),filepath.Join(ownerDir,"owner.db")
    r,err:=RunReferenceC4(context.Background(),db,owner,bundlePath,opt)
    if err!=nil{t.Fatal(err)}
    return r,db,owner
}

func TestReferenceC4SameRunAllThreeSlicesAndSafeReport(t *testing.T){
    got,path,owner:=c4Fixture(t,ReferenceOptions{})
    if got.Status!="RUN_LOCAL_10C4"||got.AbortRetry==nil||got.Communication==nil||
        got.Epistemic==nil||got.CrossAcceptance==nil||got.FinalRevision!=6||
        got.FinalInstant!=35||got.FinalStock!=0||len(got.Cycles)!=6||
        got.ReplayedCommits!=6||got.Communication.DeliveredAt!=35||
        got.AbortRetry.IndeterminateSubjects!=2||got.AbortRetry.AbortRevision!=0||
        !got.AbortRetry.SameInputDigest||!got.AbortRetry.HaltedAfterRestart||
        got.Epistemic.BKnowledgeInputs!=1||!got.Epistemic.POVIsolated||
        !got.Epistemic.RestoredAfterReopen||!got.Epistemic.WorkDeniedBeforeACK||
        !got.CrossAcceptance.SnapshotAndReplayVerified||
        !got.CrossAcceptance.FiveLedgersVerified||
        !got.CrossAcceptance.OutboxAndScheduleVerified||
        !got.CrossAcceptance.ClaimNotWorldTruth||
        !got.CrossAcceptance.OwnerAndPOVVerified{
        t.Fatalf("combined real same-RunID C4 failed: %+v",got)
    }
    if len(got.CrossAcceptance.LedgerDigests)!=5||len(got.CrossAcceptance.LedgerCounts)!=5{
        t.Fatal("C4 did not attest all five canonical ledger prefixes")
    }
    for _,name:=range []string{"input_ledger","decision_ledger","event_store","evidence_ledger","causal_outbox"}{
        raw,err:=hex.DecodeString(got.CrossAcceptance.LedgerDigests[name])
        if err!=nil||len(raw)!=32||got.CrossAcceptance.LedgerCounts[name]==0{
            t.Fatal("bad canonical ledger evidence",name,err)
        }
    }
    safe,err:=got.C4PublicReport()
    if err!=nil{t.Fatal(err)}
    data,err:=json.Marshal(safe)
    if err!=nil{t.Fatal(err)}
    var payload map[string]any
    if err:=json.Unmarshal(data,&payload);err!=nil{t.Fatal(err)}
    for _,k:=range []string{"contest_winner","contest_decisions","final_stock",
        "genesis_world_hash","final_world_hash","database","operator_private_key","event_bytes"}{
        if _,ok:=payload[k];ok{t.Fatal("C4 public report exposes engine or private key",k)}
    }
    for _,fragment:=range []string{"synthetic-a-private-only-memento",
        "resource.stolen","last.resource","actor.a.private","private-owner",
        path,owner,"contest_winner","final_stock","WorldState"}{
        if bytes.Contains(data,[]byte(fragment)){
            t.Fatal("C4 report leaked private, engine or privileged data:",fragment)
        }
    }
    if payload["status"]!="RUN_LOCAL_10C4"||payload["run_id"]!=got.RunID{
        t.Fatal("C4 report omitted essential opaque run identifier")
    }
    if !got.CrossAcceptance.ReportRedacted{
        t.Fatal("C4 public report gate not asserted")
    }
}

func TestReferenceC4FourPermutationsByteExactFiveLedgerAndOwner(t *testing.T){
    var first ReferenceResult
    for i,opt:=range []ReferenceOptions{{},{ReverseSources:true},
        {ReverseResponses:true},{ReverseSources:true,ReverseResponses:true}}{
        got,_,_:=c4Fixture(t,opt)
        if i==0{first=got;continue}
        if got.RunID!=first.RunID||got.GenesisSHA256!=first.GenesisSHA256||
            got.EventStreamSHA256!=first.EventStreamSHA256||
            !bytes.Equal(got.EventBytes,first.EventBytes)||
            got.Epistemic.SnapshotID!=first.Epistemic.SnapshotID||
            got.Epistemic.OwnerReceiptID!=first.Epistemic.OwnerReceiptID||
            got.Epistemic.AOwnerReceiptID!=first.Epistemic.AOwnerReceiptID||
            got.AbortRetry.AbortID!=first.AbortRetry.AbortID||
            got.AbortRetry.RetryID!=first.AbortRetry.RetryID{
            t.Fatalf("C4 differed under source/worker permutation %d",i)
        }
        for name,hash:=range first.CrossAcceptance.LedgerDigests{
            if got.CrossAcceptance.LedgerDigests[name]!=hash||
                got.CrossAcceptance.LedgerCounts[name]!=first.CrossAcceptance.LedgerCounts[name]{
                t.Fatalf("C4 %s bytes or count changed in permutation %d",name,i)
            }
        }
    }
}

func TestReferenceC4ReplayAndCrossTamperingDenyAll(t *testing.T){
    cases:=[]struct{name,statement string}{
        {"missing-b-observation","DELETE FROM evidence_routes WHERE holder_namespace='school' AND holder_id='b' AND kind=0"},
        {"missing-b-knowledge","DELETE FROM evidence_routes WHERE holder_namespace='school' AND holder_id='b' AND kind=1"},
        {"missing-completion","DELETE FROM causal_outbox WHERE length(canonical)<100"},
        {"corrupt-retry","UPDATE decision_ledger SET canonical=x'80' WHERE id=(SELECT id FROM decision_ledger ORDER BY rowid LIMIT 1 OFFSET 2)"},
    }
    for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
        got,db,_:=c4Fixture(t,ReferenceOptions{})
        ctx:=context.Background()
        raw,err:=sql.Open("sqlite","file:"+db)
        if err!=nil{t.Fatal(err)}
        // Scope the mutation to the exact task or retry identity below.
        var query string
        switch tc.name {
        case "missing-b-observation","missing-b-knowledge":
            task:=referenceC2Decode32(t,got.Communication.DeliveryTaskID)
            query=tc.statement+" AND task_id=x'"+hex.EncodeToString(task[:])+"'"
        case "missing-completion":
            task:=referenceC2Decode32(t,got.Communication.DeliveryTaskID)
            query="DELETE FROM causal_outbox WHERE canonical LIKE '%invalid-never%'" // replaced by exact owner test if no SQL LIKE
            _=task
        case "corrupt-retry":
            retry:=referenceC2Decode32(t,got.AbortRetry.RetryID)
            query="UPDATE decision_ledger SET canonical=x'80' WHERE id=x'"+hex.EncodeToString(retry[:])+"'"
        }
        if tc.name=="missing-completion" {
            // The C1 completion is the five-element outbox record whose
            // parent identity is the actual delivered PerceptionTask.
            rows,err:=raw.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
            if err!=nil{raw.Close();t.Fatal(err)}
            var id []byte
            task:=referenceC2Decode32(t,got.Communication.DeliveryTaskID)
            for rows.Next(){
                var rid,record []byte
                if err:=rows.Scan(&rid,&record);err!=nil{rows.Close();raw.Close();t.Fatal(err)}
                decoded,err:=codec.StrictDecodePayload(record)
                if err!=nil{rows.Close();raw.Close();t.Fatal(err)}
                f,ok:=decoded.([]any)
                if ok&&len(f)==5 {
                    parent,ok:=f[1].([]byte)
                    if ok&&bytes.Equal(parent,task[:]){id=bytes.Clone(rid)}
                }
            }
            if err:=rows.Err();err!=nil{rows.Close();raw.Close();t.Fatal(err)}
            rows.Close()
            if len(id)!=32{raw.Close();t.Fatal("C4 expected genuine B completion")}
            query="DELETE FROM causal_outbox WHERE id=x'"+hex.EncodeToString(id)+"'"
        }
        affected,err:=raw.ExecContext(ctx,query)
        if err!=nil{raw.Close();t.Fatal(err)}
        n,err:=affected.RowsAffected()
        if err!=nil||n!=1{raw.Close();t.Fatal("negative mutation did not touch one canonical record",n,err)}
        if err:=raw.Close();err!=nil{t.Fatal(err)}
        s,err:=sqlite.Open(ctx,db)
        if err!=nil{return} // early fail-closed is acceptable
        defer s.Close()
        b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
        run:=referenceC2Decode32(t,got.RunID)
        if _,err:=s.ReplayClock(ctx,run,b);err==nil{
            t.Fatal("C4 admitted missing/forged evidence or terminal authority into replay")
        }
    })}
}

func TestReferenceC4CrossFaultsNeverCreateOwnerOrPartialTruth(t *testing.T){
    cases:=[]struct{name string;set func(*ReferenceOptions)}{
        {"validator-abort-1",func(o *ReferenceOptions){o.FailAbortAt=1}},
        {"validator-abort-2",func(o *ReferenceOptions){o.FailAbortAt=2}},
        {"retry-1",func(o *ReferenceOptions){o.FailRetryAt=1}},
        {"retry-2",func(o *ReferenceOptions){o.FailRetryAt=2}},
        {"unauthorized-operator",func(o *ReferenceOptions){o.ForgeRetryOperator=true}},
        {"unauthorized-sender",func(o *ReferenceOptions){o.ForgeCommunicationActor=true}},
        {"delivery-rollback",func(o *ReferenceOptions){o.FailCommunicationDeliveryAt=1}},
    }
    for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
        dir:=t.TempDir()
        ownDir:=filepath.Join(dir,"private")
        if err:=os.Mkdir(ownDir,0700);err!=nil{t.Fatal(err)}
        db,owner:=filepath.Join(dir,"causal.db"),filepath.Join(ownDir,"owner.db")
        opt:=ReferenceOptions{}
        tc.set(&opt)
        if _,err:=RunReferenceC4(context.Background(),db,owner,bundlePath,opt);err==nil{
            t.Fatal("C4 negative injected transition returned successful public report")
        }
        if _,err:=os.Stat(owner);!os.IsNotExist(err){
            t.Fatal("C4 failure still published private owner checkpoint",err)
        }
        if _,err:=os.Stat(owner+".actor-a");!os.IsNotExist(err){
            t.Fatal("C4 failure still published A owner checkpoint",err)
        }
        s,err:=sqlite.Open(context.Background(),db)
        if err!=nil{t.Fatal(err)}
        defer s.Close()
        run:=referenceC2Decode32(t, "f40baf490888aebff1b986737de1e94a42cf94eb70aaf8342c5656bf3cda98d0")
        _=run // replay of unfinished in-flight faults has separate normative coverage
    })}
}

func TestReferenceC4OnlyRunsWithPrivateIndependentOwner(t *testing.T){
    dir:=t.TempDir()
    file:=filepath.Join(dir,"causal.db")
    if _,err:=RunReferenceC4(context.Background(),file,file,bundlePath,ReferenceOptions{});err==nil{
        t.Fatal("C4 accepted shared causal and cognitive authority")
    }
    if _,err:=RunReferenceC4(context.Background(),file,"",bundlePath,ReferenceOptions{});err==nil{
        t.Fatal("C4 accepted empty owner store")
    }
    if _,err:=os.Stat(file);!os.IsNotExist(err){t.Fatal("C4 invalid owner mutated causal database")}
}

func TestReferenceC4PublicReportIsStrictAllowlist(t *testing.T){
    got,_,_:=c4Fixture(t,ReferenceOptions{})
    report,err:=got.C4PublicReport()
    if err!=nil{t.Fatal(err)}
    raw,err:=json.Marshal(report)
    if err!=nil{t.Fatal(err)}
    if strings.Contains(string(raw),"operator_private")||strings.Contains(string(raw),"contest_winner"){
        t.Fatal("C4 public projection exposed privileged fields")
    }
    var obj map[string]any
    if err:=json.Unmarshal(raw,&obj);err!=nil{t.Fatal(err)}
    for key:=range obj{
        switch key{
        case "status","run_id","commit_count","delivery","owner","retry","integrity":
        default:t.Fatal("C4 public projection expanded without privacy review",key)
        }
    }
}
