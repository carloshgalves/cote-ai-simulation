package commit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"sync"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// Compile-time proof that the adapter uses real R22 SQLite escrow/replay
// methods, without a second checkpoint authority or canonical schema.
var _ ownercheckpoint.Escrow = (*sqlite.Store)(nil)

// This test-only owner is an in-memory remote stand-in. It owns the signer
// and bytes; the real owner provider/remote durability drill is future #58 S1.
type owner58Fixture struct {
	mu sync.Mutex
	key ed25519.PrivateKey
	actor ownercheckpoint.Scope
	bytes []byte
	persisted bool
	restored bool
	dropAck bool
}

func (o *owner58Fixture) PersistAndSign(_ context.Context,
 req ownercheckpoint.PersistRequest)(ownercheckpoint.SignedReceipt,error){
	o.mu.Lock();defer o.mu.Unlock()
	if req.Scope!=o.actor{return ownercheckpoint.SignedReceipt{},errors.New("foreign actor persisted")}
	if o.persisted&&!bytes.Equal(o.bytes,req.Checkpoint.Content){
		return ownercheckpoint.SignedReceipt{},errors.New("owner content conflict")
	}
	o.bytes=bytes.Clone(req.Checkpoint.Content)
	o.persisted=true
	raw,err:=sqlite.CognitiveCheckpointSigningBytesV1(req.Scope.Run,req.Checkpoint,"owner-58-cut")
	if err!=nil{return ownercheckpoint.SignedReceipt{},err}
	return ownercheckpoint.SignedReceipt{
		Locator:"owner-58-cut",Signature:ed25519.Sign(o.key,raw),
	},nil
}
func (o *owner58Fixture) Restore(_ context.Context,
 req ownercheckpoint.RestoreRequest)(ownercheckpoint.RestoredContent,error){
	o.mu.Lock();defer o.mu.Unlock()
	if req.Scope!=o.actor||req.Locator!="owner-58-cut"||!o.persisted{
		return ownercheckpoint.RestoredContent{},errors.New("wrong external memory locator")
	}
	if identity.OpaqueContentHash(o.bytes)!=req.ContentHash{
		return ownercheckpoint.RestoredContent{},errors.New("external memory digest differs")
	}
	o.restored=true
	return ownercheckpoint.RestoredContent{Locator:req.Locator,Content:bytes.Clone(o.bytes)},nil
}
func (o *owner58Fixture) Acknowledge(_ context.Context,
 ack ownercheckpoint.RestoreAck)error{
	o.mu.Lock();defer o.mu.Unlock()
	if o.dropAck{return errors.New("durable owner ACK lost")}
	if !o.restored||ack.Scope!=o.actor||ack.Locator!="owner-58-cut"||
		identity.OpaqueContentHash(o.bytes)!=ack.ContentHash{
		return errors.New("owner ACK does not match restored bytes")
	}
	return nil
}

func TestCSFRV18R58S0RealSQLiteEscrowRestoreACKNoWorkBeforeRecovery(t *testing.T){
	ctx:=context.Background()
	_,store,_,run,bundle,path:=testFixture(t)
	scope:=ownercheckpoint.Scope{
		Run:[32]byte(run),Namespace:"cognition",Actor:"owner-58",
	}
	if err:=store.RegisterCognitiveActor(ctx,scope.Run,scope.Namespace,scope.Actor,bundle);err!=nil{t.Fatal(err)}
	public,private,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	if err:=store.RegisterCognitiveOwnerKeyV1(ctx,scope.Run,scope.Namespace,scope.Actor,public,bundle);err!=nil{t.Fatal(err)}
	state,err:=store.LoadClock(ctx,scope.Run)
	if err!=nil{t.Fatal(err)}
	cp:=sqlite.SnapshotCheckpoint{
		ActorNamespace:scope.Namespace,ActorID:scope.Actor,
		StoreID:"owner-memory-58",SchemaVersion:1,
		Content:[]byte("opaque owner-private checkpoint"),
		Producer:"owner-58",ProducerVersion:1,
		CoversInstant:int64(state.Instant),CoversRevision:uint64(state.Revision),
	}
	owner:=&owner58Fixture{key:private,actor:scope}
	gate,err:=ownercheckpoint.NewGate(owner,store,bundle)
	if err!=nil{t.Fatal(err)}
	receipt,err:=gate.Stage(ctx,scope,cp)
	if err!=nil{t.Fatal("durable-before-escrow:",err)}
	if receipt==([32]byte{}){t.Fatal("owner signed receipt identity absent")}
	snap,err:=store.CreateSnapshotV1(ctx,scope.Run,bundle,[]sqlite.SnapshotCheckpoint{cp})
	if err!=nil{t.Fatal("signed escrow did not publish a snapshot:",err)}
	work:=0
	callback:=func(context.Context)error{work++;return nil}
	if err:=gate.RunWork(ctx,scope,snap.ID,receipt,callback);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged)||work!=0{
		t.Fatal("WORK executed before owner Restore/ACK:",err)
	}
	owner.dropAck=true
	if err:=gate.RestoreAndAcknowledge(ctx,scope,snap.ID,receipt);err==nil{
		t.Fatal("lost external ACK authorized new WORK")
	}
	if err:=gate.RunWork(ctx,scope,snap.ID,receipt,callback);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged)||work!=0{
		t.Fatal("lost owner ACK bypassed gate:",err)
	}
	owner.dropAck=false
	if err:=gate.RestoreAndAcknowledge(ctx,scope,snap.ID,receipt);err!=nil{
		t.Fatal("exact signed escrow did not rehydrate owner:",err)
	}
	if err:=gate.RunWork(ctx,scope,snap.ID,receipt,callback);err!=nil||work!=1{
		t.Fatal("acked exact owner failed authorized work:",err)
	}
	// A new process cannot inherit an ACK from a previous process.
	reopened,err:=sqlite.Open(ctx,path)
	if err!=nil{t.Fatal(err)}
	defer reopened.Close()
	restarted,err:=ownercheckpoint.NewGate(owner,reopened,bundle)
	if err!=nil{t.Fatal(err)}
	if err:=restarted.RunWork(ctx,scope,snap.ID,receipt,callback);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged)||work!=1{
		t.Fatal("new process inherited old owner ACK:",err)
	}
	if err:=restarted.RestoreAndAcknowledge(ctx,scope,snap.ID,receipt);err!=nil{
		t.Fatal("reopened real escrow did not revalidate and restore:",err)
	}
	if err:=restarted.RunWork(ctx,scope,snap.ID,receipt,callback);err!=nil||work!=2{
		t.Fatal("reopened verified checkpoint could not authorize work:",err)
	}
	// Multiple individually valid actor/snapshot identities are not enough.
	other:=scope;other.Actor="wrong-owner"
	if err:=restarted.RunWork(ctx,other,snap.ID,receipt,callback);!errors.Is(err,ownercheckpoint.ErrNotAcknowledged)||work!=2{
		t.Fatal("cross-actor owner gate reused signed checkpoint:",err)
	}
}
