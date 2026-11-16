package commit

import (
    "bytes"
    "context"
    "database/sql"
    "encoding/hex"
    "path/filepath"
    "testing"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    _ "modernc.org/sqlite"
)

// C3 must reuse the original B/C1 causal genesis, two actors, source cohort,
// original authenticated AdmissionFence and a genuine validator failure.
func TestReferenceC3IndeterminateAbortRetryAndC1Continuation(t *testing.T) {
    ctx:=context.Background()
    file:=filepath.Join(t.TempDir(),"c3.db")
    res,err:=RunReference(ctx,file,bundlePath,ReferenceOptions{
        IncludeAbortRetry:true,IncludeCommunication:true,
    })
    if err!=nil{t.Fatal(err)}
    p:=res.AbortRetry
    if p==nil||res.Status!="RUN_LOCAL_10C3"||res.FinalRevision!=6||
        res.FinalStock!=0||len(res.Cycles)!=6||res.ReplayedCommits!=6||
        !p.ValidatorIndeterminate||p.IndeterminateSubjects!=2||
        !p.HaltedAfterRestart||!p.UnauthorizedFenceDenied||
        !p.RetryAuthorizedAfterRestart||!p.SameInputDigest||
        !p.SameMembership||!p.SamePolicyAndClock||!p.ReplayVerified||
        p.AbortRevision!=0||p.AbortEvents!=0||p.AbortCommitCount!=0||
        p.RetryAttempt!=1||p.AbortID==""||p.RetryID==""||
        p.AbortID==p.RetryID||p.PreviousFenceID==p.RetryFenceID{
        t.Fatalf("C3 failed authentic terminal+recovery gates: %+v",res)
    }
    if res.Communication==nil||res.Communication.DeliveredAt!=35{
        t.Fatal("C3 did not continue real same-run delayed Claim delivery")
    }
    store,err:=sqlite.Open(ctx,file);if err!=nil{t.Fatal(err)};defer store.Close()
    runBytes,err:=hex.DecodeString(res.RunID);if err!=nil{t.Fatal(err)}
    var run [32]byte;copy(run[:],runBytes)
    ctl,err:=store.LoadCycleControl(ctx,run)
    if err!=nil||ctl[0]!=uint64(0){t.Fatalf("C3 terminal commit not IDLE: %v %v",ctl,err)}
    bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
    replay,err:=store.ReplayClock(ctx,run,bundle)
    if err!=nil||replay.ReplayedCommits!=6{t.Fatalf("C3 replay: %v %+v",err,replay)}
}

func TestReferenceC3ByteExactAcrossFourWorkerPermutations(t *testing.T){
    var first ReferenceResult
    for i,opt:=range []ReferenceOptions{
        {IncludeAbortRetry:true},{IncludeAbortRetry:true,ReverseSources:true},
        {IncludeAbortRetry:true,ReverseResponses:true},
        {IncludeAbortRetry:true,ReverseSources:true,ReverseResponses:true},
    }{
        got,err:=RunReference(context.Background(),filepath.Join(t.TempDir(),"c3.db"),bundlePath,opt)
        if err!=nil{t.Fatal(i,err)}
        if i==0{first=got;continue}
        if got.RunID!=first.RunID||got.GenesisSHA256!=first.GenesisSHA256||
            got.EventStreamSHA256!=first.EventStreamSHA256||
            got.AbortRetry.AbortID!=first.AbortRetry.AbortID||
            got.AbortRetry.RetryID!=first.AbortRetry.RetryID||
            !bytes.Equal(got.EventBytes,first.EventBytes){
            t.Fatalf("worker source order altered C3 canonical bytes at %d",i)
        }
    }
}

func TestReferenceC3AbortAndRetryFaultInjectionAtomic(t *testing.T){
    for _,kind:=range []string{"abort","retry"}{
        for _,stage:=range []uint8{1,2}{
            t.Run(kind+"-step-"+string(rune('0'+stage)),func(t *testing.T){
                file:=filepath.Join(t.TempDir(),"c3.db")
                opt:=ReferenceOptions{IncludeAbortRetry:true}
                if kind=="abort"{opt.FailAbortAt=stage}else{opt.FailRetryAt=stage}
                _,err:=RunReference(context.Background(),file,bundlePath,opt)
                if err==nil{t.Fatal("C3 injected SQL failure was ignored")}
                db,err:=sql.Open("sqlite","file:"+file);if err!=nil{t.Fatal(err)};defer db.Close()
                for _,table:=range []string{"committed_cycles","event_store"}{
                    var n int
                    if err:=db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n);err!=nil||n!=0{
                        t.Fatalf("C3 %s rollback exposed partial %s: %d %v",kind,table,n,err)
                    }
                }
                var revs int
                if err:=db.QueryRow("SELECT COUNT(*) FROM world_revisions").Scan(&revs);err!=nil||revs!=1{
                    t.Fatalf("C3 %s rollback emitted WorldRevision %d %v",kind,revs,err)
                }
                var count int
                if err:=db.QueryRow("SELECT COUNT(*) FROM decision_ledger").Scan(&count);err!=nil{t.Fatal(err)}
                want:=1
                if kind=="retry"{want=2}
                if count!=want{t.Fatalf("%s leaked provisional decision journal %d != %d",kind,count,want)}
                var raw []byte
                if err:=db.QueryRow("SELECT canonical FROM cycle_control_state").Scan(&raw);err!=nil{t.Fatal(err)}
                value,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
                got:=value.([]any)[0].(uint64)
                wantStatus:=uint64(1)
                if kind=="retry"{wantStatus=3}
                if got!=wantStatus{t.Fatalf("%s got control state %d, wanted %d",kind,got,wantStatus)}
            })
        }
    }
}

func TestReferenceC3ForgedAuthorityDenied(t *testing.T){
    file:=filepath.Join(t.TempDir(),"forged.db")
    _,err:=RunReference(context.Background(),file,bundlePath,
        ReferenceOptions{IncludeAbortRetry:true,ForgeRetryOperator:true})
    if err==nil{t.Fatal("unauthorized retry signature accepted")}
    db,err:=sql.Open("sqlite","file:"+file);if err!=nil{t.Fatal(err)};defer db.Close()
    var raw []byte
    if err:=db.QueryRow("SELECT canonical FROM cycle_control_state").Scan(&raw);err!=nil{t.Fatal(err)}
    fields,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
    if fields.([]any)[0]!=uint64(3){t.Fatal("spoofed operator cleared HALTED_ON_ABORT")}
    var n int
    if err:=db.QueryRow("SELECT COUNT(*) FROM committed_cycles").Scan(&n);err!=nil||n!=0{
        t.Fatal("spoofed retry published WORK",n,err)
    }
}
