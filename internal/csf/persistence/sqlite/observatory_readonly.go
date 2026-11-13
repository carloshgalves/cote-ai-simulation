package sqlite

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "net/url"
    "path/filepath"
    "reflect"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    modernsqlite "modernc.org/sqlite"
)

// ObservatoryReader owns a separate, enforced read-only SQLite connection.
// It does not embed Store or expose any method that can mutate causal data.
// All normative genesis, replay and CycleControl verification is reused from
// Store internally; read-only mode and query_only independently reject writes.
type ObservatoryReader struct {
    store *Store
    bundle *policy.Bundle
}

var _ observatory.Queries = (*ObservatoryReader)(nil)

func OpenObservatory(ctx context.Context, path string, bundle *policy.Bundle) (*ObservatoryReader,error) {
    if bundle==nil { return nil,errors.New("observatory needs an installed causal bundle") }
    if err:=bundle.VerifyInstalledBundle();err!=nil{return nil,fmt.Errorf("observatory policy: %w",err)}
    absolute,err:=filepath.Abs(path)
    if err!=nil{return nil,err}
    uri:=&url.URL{Scheme:"file",Path:filepath.ToSlash(absolute)}
    q:=url.Values{}
    // DO NOT use immutable=1: it ignores the live WAL and can silently
    // return a stale run. mode=ro is enforced by the SQLite pager.
    q.Set("mode","ro")
    q.Add("_pragma","query_only(ON)")
    q.Add("_pragma","synchronous(FULL)")
    q.Add("_pragma","foreign_keys(ON)")
    uri.RawQuery=q.Encode()
    connector,err:=modernsqlite.NewConnector(uri.String())
    if err!=nil{return nil,err}
    db:=sql.OpenDB(connector)
    // One persistent connection stabilizes PRAGMA data_version checks, which
    // are connection-relative and detect concurrent writes by other handles.
    db.SetMaxOpenConns(1)
    db.SetMaxIdleConns(1)
    s:=&Store{db:db}
    failed:=func(err error)(*ObservatoryReader,error) { _=db.Close();return nil,err }
    if err:=db.PingContext(ctx);err!=nil{return failed(fmt.Errorf("open existing CSF database in read-only mode: %w",err))}
    if err:=s.verifyRuntime(ctx);err!=nil{return failed(err)}
    var only int
    if err:=db.QueryRowContext(ctx,"PRAGMA query_only").Scan(&only);err!=nil{return failed(err)}
    if only!=1{return failed(errors.New("observatory SQLite connection is not query_only"))}
    // Deliberately no Store.createSchema and no writable Store export.
    return &ObservatoryReader{store:s,bundle:bundle},nil
}

func (r *ObservatoryReader) Close() error {
    if r==nil||r.store==nil {return errors.New("nil observatory reader")}
    return r.store.Close()
}

func (r *ObservatoryReader) dataVersion(ctx context.Context)(int64,error) {
    var version int64
    err:=r.store.db.QueryRowContext(ctx,"PRAGMA data_version").Scan(&version)
    return version,err
}

// ReadRun reuses the existing replay verifier rather than implementing a
// second causal engine. If another connection modifies the database during
// verification, fail closed instead of returning a mixed old/new projection.
func (r *ObservatoryReader) ReadRun(ctx context.Context,run [32]byte)(observatory.RunState,error) {
    var zero observatory.RunState
    if r==nil||r.store==nil||r.bundle==nil{return zero,errors.New("observatory reader is not initialized")}
    if err:=r.bundle.VerifyInstalledBundle();err!=nil{return zero,err}
    versionBefore,err:=r.dataVersion(ctx)
    if err!=nil{return zero,err}
    first,err:=r.loadReadRun(ctx,run)
    if err!=nil{return zero,err}
    proof,err:=r.store.ReplayClock(ctx,run,r.bundle)
    if err!=nil{return zero,fmt.Errorf("observatory causal replay verification: %w",err)}
    if first.Clock!=proof.ReconstructedClock{
        return zero,errors.New("observatory run clock diverged from authenticated replay")
    }
    last,err:=r.loadReadRun(ctx,run)
    if err!=nil{return zero,err}
    versionAfter,err:=r.dataVersion(ctx)
    if err!=nil{return zero,err}
    if versionBefore!=versionAfter||!reflect.DeepEqual(first,last){
        return zero,errors.New("observatory source changed during verified query")
    }
    return last,nil
}

func (r *ObservatoryReader) loadReadRun(ctx context.Context,run [32]byte)(observatory.RunState,error) {
    out:=observatory.RunState{Run:run}
    tx,err:=r.store.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
    if err!=nil{return observatory.RunState{},err}
    defer tx.Rollback()
    // The causal SQLite file holds one world. Never join a status from one
    // run with a world revision whose identity belongs to another genesis.
    var count int
    if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM genesis_records").Scan(&count);err!=nil{return observatory.RunState{},err}
    if count!=1{return observatory.RunState{},errors.New("observatory requires exactly one causal genesis")}
    var clockRaw []byte
    if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",run[:]).Scan(&clockRaw);err!=nil{
        return observatory.RunState{},fmt.Errorf("observatory clock authority absent: %w",err)
    }
    out.Clock,err=clock.Decode(clockRaw)
    if err!=nil{return observatory.RunState{},err}
    var worldHash []byte
    if err:=tx.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=?",uint64(out.Clock.Revision)).Scan(&worldHash);err!=nil{
        return observatory.RunState{},fmt.Errorf("observatory world revision absent: %w",err)
    }
    if len(worldHash)!=32{return observatory.RunState{},errors.New("invalid authoritative world hash")}
    copy(out.WorldHash[:],worldHash)
    ctl,err:=cycleControl(ctx,tx,run)
    if err!=nil{return observatory.RunState{},err}
    out.Control,err=observatoryControl(ctl)
    if err!=nil{return observatory.RunState{},err}
    if err:=tx.Commit();err!=nil{return observatory.RunState{},err}
    return out,nil
}

func observatoryControl(fields []any)(observatory.CycleControl,error) {
    out:=observatory.CycleControl{Status:observatory.IDLE}
    if fields==nil{return out,nil}
    if len(fields)!=7{return out,errors.New("invalid CycleControlState arity")}
    kind,ok:=fields[0].(uint64)
    if !ok{return out,errors.New("invalid CycleControlState kind")}
    switch kind{
    case 0:out.Status=observatory.IDLE
    case 1:out.Status=observatory.ATTEMPT_IN_FLIGHT
    case 2:out.Status=observatory.RETRY_AUTHORIZED
    case 3:out.Status=observatory.HALTED_ON_ABORT
    default:return out,errors.New("unsupported CycleControlState kind")
    }
    next,ok:=fields[6].(uint64)
    if !ok{return out,errors.New("invalid CycleControlState next attempt")}
    out.NextAttempt=next
    optional:=func(v any)(any,bool,error){
        arr,ok:=v.([]any)
        if !ok{return nil,false,errors.New("invalid optional CycleControlState field")}
        if len(arr)==1&&arr[0]==uint64(0){return nil,false,nil}
        if len(arr)==2&&arr[0]==uint64(1){return arr[1],true,nil}
        return nil,false,errors.New("unknown optional CycleControlState variant")
    }
    cycle,hasCycle,err:=optional(fields[1])
    if err!=nil{return out,err}
    ordinal,hasOrdinal,err:=optional(fields[2])
    if err!=nil{return out,err}
    if hasCycle!=hasOrdinal||(kind!=0&&!hasCycle)||(kind==0&&hasCycle){
        return out,errors.New("missing/forged CycleControlState cycle or ordinal")
    }
    if hasCycle{
        bytes,ok:=cycle.([]byte)
        if !ok||len(bytes)!=32{return out,errors.New("invalid CycleControlState cycle id")}
        var id [32]byte
        copy(id[:],bytes)
        out.CycleID=&id
        attempt,ok:=ordinal.(uint64)
        if !ok{return out,errors.New("invalid CycleControlState attempt")}
        out.Attempt=&attempt
    }
    return out,nil
}
