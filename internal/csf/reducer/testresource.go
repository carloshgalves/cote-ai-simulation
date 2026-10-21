// Package reducer contains pure, versioned world-state transitions.
// A reducer has no store, wall clock, RNG, LLM or observation permissions.
package reducer

import (
 "bytes"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

const StockSchemaID="cote.csf.schema.typed-value"
const StockDomainTag="cote.csf.hash.world-state"
const StockVariant="testresource.stock.v1"

var (
 ErrInvalidStock=errors.New("unregistered testresource WorldState")
 ErrInsufficientStock=errors.New("resource stock invariant violated")
)
type Stock struct{Available uint64}

// ValidateStockPayload binds the closed testresource extension to the exact
// pinable world-state schema already registered in canonical_bytes_v1.
// No arbitrary typed-value content is treated as inventory.
func ValidateStockPayload(value any)error{
 p,ok:=value.([]any)
 if !ok||len(p)!=3||p[0]!=StockVariant||p[1]!=uint64(1){return ErrInvalidStock}
 rows,ok:=p[2].([]any)
 if !ok||len(rows)!=1{return ErrInvalidStock}
 item,ok:=rows[0].([]any)
 if !ok||len(item)!=3||item[0]!="inventory"||item[1]!="last.resource"{return ErrInvalidStock}
 if _,ok:=item[2].(uint64);!ok{return ErrInvalidStock}
 return nil
}
func EncodeStock(stock Stock)([]any,domain.Digest,error){
 payload:=[]any{StockVariant,uint64(1),[]any{[]any{"inventory","last.resource",stock.Available}}}
 inner,err:=codec.CanonicalBytes(StockDomainTag,StockSchemaID,1,payload)
 if err!=nil{return nil,domain.Digest{},err}
 dg:=identity.OpaqueContentHash(inner)
 return []any{StockSchemaID,uint64(1),inner,dg[:]},dg,nil
}
func DecodeStock(value any)(Stock,error){
 var stock Stock
 typed,ok:=value.([]any)
 if !ok||len(typed)!=4||typed[0]!=StockSchemaID||typed[1]!=uint64(1){return stock,ErrInvalidStock}
 raw,ok:=typed[2].([]byte);if !ok{return stock,ErrInvalidStock}
 hash,ok:=typed[3].([]byte);if !ok||len(hash)!=32{return stock,ErrInvalidStock}
 env,err:=codec.StrictDecodeEnvelope(raw)
 if err!=nil||env.DomainTag!=StockDomainTag||env.SchemaID!=StockSchemaID||env.SchemaVersion!=1{
  return stock,ErrInvalidStock
 }
 if err:=ValidateStockPayload(env.Value);err!=nil{return stock,err}
 dg:=identity.OpaqueContentHash(raw)
 if !bytes.Equal(hash,dg[:]){return stock,ErrInvalidStock}
 items:=env.Value.([]any)[2].([]any)
 stock.Available=items[0].([]any)[2].(uint64)
 return stock,nil
}

// Consume is a total, deterministic, pure resource reducer. It neither
// selects a winning candidate nor retries. The terminal coordinator must
// prove accepted claims and call this with the canonical ordered winner set.
func Consume(before []any,amount uint64)([]any,domain.Digest,error){
 var zero domain.Digest
 if amount==0{return nil,zero,fmt.Errorf("%w: zero resource consumption",ErrInvalidStock)}
 stock,err:=DecodeStock(before);if err!=nil{return nil,zero,err}
 if stock.Available<amount{return nil,zero,ErrInsufficientStock}
 stock.Available-=amount
 return EncodeStock(stock)
}
