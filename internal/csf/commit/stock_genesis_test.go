package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

func stockGenesis(t *testing.T,b *policy.Bundle)*genesis.Genesis{
 t.Helper()
 g:=validGenesis(t,b)
 decoded,err:=codec.StrictDecodeEnvelope(g.CanonicalBytes);if err!=nil{t.Fatal(err)}
 fields:=append([]any(nil),decoded.Value.([]any)...)
 stock,_,err:=reducer.EncodeStock(reducer.Stock{Available:1});if err!=nil{t.Fatal(err)}
 fields[15]=[]any{[]any{reducer.StockSchemaID,uint64(1)}}
 fields[16]=stock
 encoded,err:=codec.CanonicalBytes(decoded.DomainTag,decoded.SchemaID,decoded.SchemaVersion,fields)
 if err!=nil{t.Fatal(err)}
 checked,err:=genesis.ValidateCanonical(encoded,b)
 if err!=nil{t.Fatalf("versioned testresource WorldState refused by genesis: %v",err)}
 return checked
}
func TestPinnedStockGenesisRestoresAuthoritativeInventory(t *testing.T){
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=stockGenesis(t,bundle)
 store,err:=sqlite.Open(ctx,t.TempDir()+"/world-stock.db");if err!=nil{t.Fatal(err)}
 defer store.Close()
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 before,hash,err:=store.LoadWorldState(ctx,0);if err!=nil{t.Fatal(err)}
 stock,err:=reducer.DecodeStock(before);if err!=nil||stock.Available!=1{t.Fatalf("stock not recoverable from pinned Genesis: %+v %v",stock,err)}
 gd:=g.Payload[16].([]any)[3].([]byte)
 if len(gd)!=32{t.Fatal("genesis hash missing")}
 for i:=range hash{if hash[i]!=gd[i]{t.Fatal("initial stock hash not pinned")}}
}
