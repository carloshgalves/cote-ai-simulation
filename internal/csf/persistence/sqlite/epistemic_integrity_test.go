package sqlite

import (
    "bytes"
    "context"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestEvidenceIsolationDoesNotBlockUnrelatedEmptyTask(t *testing.T) {
    ctx:=context.Background()
    bundle,err:=policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
    if err!=nil{t.Fatal(err)}
    store,err:=Open(ctx,t.TempDir()+"/addressed.db")
    if err!=nil{t.Fatal(err)}
    defer store.Close()

    var secretTask, visibleTask, eventID, eventDigest [32]byte
    secretTask[0],visibleTask[0],eventID[0],eventDigest[0]=1,2,3,4
    actor:=[]any{"school","A"}
    source:=[]any{eventID[:],eventDigest[:]}
    chain:=[]any{[]any{uint64(0),eventID[:],eventDigest[:]}}
    resolver:=[]any{"validators-v1",uint64(1),bytes.Repeat([]byte{7},32)}
    d:=identity.New(bundle)
    observationID,_,err:=d.Sum("cote.csf.id.observation",
        "cote.csf.schema.observation-id-preimage",1,
        []any{visibleTask[:],actor,uint64(0),uint64(0)})
    if err!=nil{t.Fatal(err)}
    observationBody:=[]any{observationID[:],visibleTask[:],actor,
        uint64(0),uint64(0),int64(123),[]any{source},uint64(0),
        []any{uint64(0)},[]any{},[]any{},[]any{},chain,resolver}
    observationDigest,_,err:=d.Sum("cote.csf.digest.observation",
        "cote.csf.schema.observation.body",1,observationBody)
    if err!=nil{t.Fatal(err)}
    observationRaw,err:=codec.CanonicalPayload(append(observationBody,observationDigest[:]))
    if err!=nil{t.Fatal(err)}

    ref:=[]any{uint64(16),observationID[:],observationDigest[:]}
    inputID,_,err:=d.Sum("cote.csf.id.knowledge-input",
        "cote.csf.schema.knowledge-input-id-preimage",1,
        []any{observationID[:],actor,uint64(0),uint64(0),uint64(0)})
    if err!=nil{t.Fatal(err)}
    inputBody:=[]any{inputID[:],uint64(0),uint64(0),actor,int64(123),
        uint64(0),"direct",ref,[]any{},chain}
    inputDigest,_,err:=d.Sum("cote.csf.digest.knowledge-input",
        "cote.csf.schema.knowledge-input.body",1,inputBody)
    if err!=nil{t.Fatal(err)}
    inputRaw,err:=codec.CanonicalPayload(append(inputBody,inputDigest[:]))
    if err!=nil{t.Fatal(err)}
    _,err=store.db.ExecContext(ctx,"INSERT INTO evidence_ledger(id,canonical) VALUES(?,?)",
        observationID[:],observationRaw)
    if err!=nil{t.Fatal(err)}
    _,err=store.db.ExecContext(ctx,"INSERT INTO evidence_ledger(id,canonical) VALUES(?,?)",
        inputID[:],inputRaw)
    if err!=nil{t.Fatal(err)}
    tx,err:=store.db.BeginTx(ctx,nil)
    if err!=nil{t.Fatal(err)}
    defer tx.Rollback()
    if err:=assertNoAddressedOutputs(ctx,tx,secretTask,bundle);err!=nil{
        t.Fatal("another task's addressed evidence blocked invisible task",err)
    }
    if err:=assertNoAddressedOutputs(ctx,tx,visibleTask,bundle);err==nil{
        t.Fatal("task with an observation was eligible for empty completion")
    }
}

func TestEvidenceReconciliationRejectsDanglingKnowledgeInput(t *testing.T) {
    ctx:=context.Background()
    bundle,err:=policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
    if err!=nil{t.Fatal(err)}
    store,err:=Open(ctx,t.TempDir()+"/dangling.db")
    if err!=nil{t.Fatal(err)}
    defer store.Close()
    var task,missing [32]byte
    task[0],missing[0]=1,2
    actor:=[]any{"school","B"}
    d:=identity.New(bundle)
    id,_,err:=d.Sum("cote.csf.id.knowledge-input",
        "cote.csf.schema.knowledge-input-id-preimage",1,
        []any{missing[:],actor,uint64(0),uint64(0),uint64(0)})
    if err!=nil{t.Fatal(err)}
    chain:=[]any{[]any{uint64(0),missing[:],missing[:]}}
    body:=[]any{id[:],uint64(0),uint64(0),actor,int64(0),uint64(0),
        "direct",[]any{uint64(16),missing[:],missing[:]},[]any{},chain}
    digest,_,err:=d.Sum("cote.csf.digest.knowledge-input",
        "cote.csf.schema.knowledge-input.body",1,body)
    if err!=nil{t.Fatal(err)}
    raw,err:=codec.CanonicalPayload(append(body,digest[:]))
    if err!=nil{t.Fatal(err)}
    _,err=store.db.ExecContext(ctx,"INSERT INTO evidence_ledger(id,canonical) VALUES(?,?)",id[:],raw)
    if err!=nil{t.Fatal(err)}
    tx,err:=store.db.BeginTx(ctx,nil)
    if err!=nil{t.Fatal(err)}
    defer tx.Rollback()
    if err:=assertNoAddressedOutputs(ctx,tx,task,bundle);err==nil{
        t.Fatal("dangling KnowledgeInput accepted as unrelated evidence")
    }
}
