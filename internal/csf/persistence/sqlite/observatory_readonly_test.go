package sqlite

import (
    "context"
    "database/sql"
    "os"
    "path/filepath"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV19O1SQLiteReadOnlyModeAndIndependentQueryOnlyDefense(t *testing.T){
    ctx:=context.Background()
    dir:=t.TempDir()
    path:=filepath.Join(dir,"authority.db")
    writer,err:=Open(ctx,path)
    if err!=nil{t.Fatal(err)}
    if err:=writer.Close();err!=nil{t.Fatal(err)}
    b,err:=policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
    if err!=nil{t.Fatal(err)}
    r,err:=OpenObservatory(ctx,path,b)
    if err!=nil{t.Fatal(err)}
    defer r.Close()
    var flag int
    if err:=r.store.db.QueryRowContext(ctx,"PRAGMA query_only").Scan(&flag);err!=nil||flag!=1{
        t.Fatalf("query_only not enabled on the actual connection: %d %v",flag,err)
    }
    if _,err:=r.store.db.ExecContext(ctx,"INSERT INTO input_ledger(id,canonical) VALUES(zeroblob(32),?)",[]byte{0x01});err==nil{
        t.Fatal("read-only connection was allowed to INSERT")
    }
    if _,err:=r.store.db.ExecContext(ctx,"CREATE TABLE o1_write_bypass (x INTEGER)");err==nil{
        t.Fatal("read-only connection was allowed to change schema")
    }
    // Even if a future internal caller disables the advisory connection
    // query_only pragma, SQLite mode=ro must still reject all file writes.
    if _,err:=r.store.db.ExecContext(ctx,"PRAGMA query_only=OFF");err!=nil{t.Fatal(err)}
    if _,err:=r.store.db.ExecContext(ctx,"INSERT INTO input_ledger(id,canonical) VALUES(zeroblob(32),?)",[]byte{0x01});err==nil{
        t.Fatal("pager was writable after query_only was disabled: mode=ro missing")
    }
    var rows int
    audit,err:=sql.Open("sqlite",path)
    if err!=nil{t.Fatal(err)}
    defer audit.Close()
    if err:=audit.QueryRowContext(ctx,"SELECT count(*) FROM input_ledger").Scan(&rows);err!=nil||rows!=0 {
        t.Fatalf("read-only mode leaked a write: %d %v",rows,err)
    }
    if _,err:=os.Stat(path);err!=nil{t.Fatal("read-only query corrupted/removed sqlite database:",err)}
}
