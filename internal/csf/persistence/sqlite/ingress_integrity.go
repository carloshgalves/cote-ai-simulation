package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ingressguard"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type ingressQuery interface{
 QueryContext(context.Context,string,...any)(*sql.Rows,error)
 QueryRowContext(context.Context,string,...any)*sql.Row
}
type ingressProjection struct{
 source string
 seq uint64
 record [32]byte
 raw []byte
}

// The receipt index/cursor is a projection of canonical InputLedger records.
// Keep it checked inside the writer transaction and during replay, rather than
// letting SQL-derived cursor state become a second ingress authority.
func verifyIngressProjections(ctx context.Context,q ingressQuery)error{
 // Raw adapter atomicity tests without a Genesis/Clock/Fence are intentionally
 // not executable simulations and use opaque record bytes.
 var authorityCount int
 if err:=q.QueryRowContext(ctx,"SELECT (SELECT count(*) FROM genesis_records)+(SELECT count(*) FROM clock_states)+(SELECT count(*) FROM decision_ledger)").Scan(&authorityCount);err!=nil{return err}
 if authorityCount==0{return nil}
 declared:=map[string]bool{}
 canonical:=map[string]ingressProjection{}
 receiptRecordIDs:=map[[32]byte]struct{}{}
 rows,err:=q.QueryContext(ctx,"SELECT id,canonical FROM input_ledger ORDER BY rowid")
 if err!=nil{return err}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return err}
  if len(id)!=32{rows.Close();return errors.New("ingress canonical record has invalid id")}
  v,err:=codec.StrictDecodePayload(raw)
  if err==nil{
   f,ok:=v.([]any)
   if ok&&len(f)==3&&f[0]=="source-declaration-v1"{
    source,ok:=f[2].(string)
    if !ok||source==""||declared[source]{rows.Close();return errors.New("duplicate or invalid source declaration")}
    h:=identity.OpaqueContentHash(raw)
    if !bytes.Equal(id,h[:]){rows.Close();return errors.New("source declaration does not match identity")}
    declared[source]=true
   }
  }
  env,err:=codec.StrictDecodeEnvelope(raw)
  if err!=nil||env.DomainTag!="cote.csf.record.source-ingress-receipt"{continue}
  if env.SchemaID!="cote.csf.schema.source-ingress-receipt"||env.SchemaVersion!=1{rows.Close();return errors.New("ingress receipt uses unknown schema")}
  h:=identity.OpaqueContentHash(raw)
  if !bytes.Equal(id,h[:]){rows.Close();return errors.New("ingress receipt id invalid")}
  f,ok:=env.Value.([]any)
  if !ok||len(f)!=3{rows.Close();return errors.New("ingress receipt body malformed")}
  source,ok:=f[0].(string);if !ok||source==""{rows.Close();return errors.New("ingress receipt source invalid")}
  seq,ok:=f[1].(uint64);if !ok{rows.Close();return errors.New("ingress receipt sequence invalid")}
  ref,ok:=f[2].([]any);if !ok||len(ref)!=3{rows.Close();return errors.New("ingress receipt reference malformed")}
  kind,ok:=ref[0].(uint64);if !ok||(kind!=1&&kind!=2){rows.Close();return errors.New("ingress receipt kind invalid")}
  idRef,ok:=ref[1].([]byte);if !ok||len(idRef)!=32{rows.Close();return errors.New("ingress receipt record id invalid")}
  digest,ok:=ref[2].([]byte);if !ok||len(digest)!=32{rows.Close();return errors.New("ingress receipt digest invalid")}
  key:=fmt.Sprintf("%s/%d",source,seq)
  if _,exists:=canonical[key];exists{rows.Close();return errors.New("duplicate canonical ingress receipt coordinate")}
  var rid [32]byte;copy(rid[:],idRef)
  if _,exists:=receiptRecordIDs[rid];exists{
   rows.Close();return errors.New("duplicate ingress receipts for one causal record ID")
  }
  receiptRecordIDs[rid]=struct{}{}
  canonical[key]=ingressProjection{source:source,seq:seq,record:rid,raw:raw}
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 expected:=map[string]uint64{}
 for _,receipt:=range canonical{
  if !declared[receipt.source]{return errors.New("ingress receipt refers to undeclared source")}
  expected[receipt.source]++
 }
 // Projection equality alone must not certify an internally consistent but
 // non-gapless set of source receipt keys.
 for source,count:=range expected{
  for sequence:=uint64(0);sequence<count;sequence++{
   if _,ok:=canonical[fmt.Sprintf("%s/%d",source,sequence)];!ok{
    return errors.New("canonical source receipt sequence contains a gap")
   }
  }
 }
 indices,err:=q.QueryContext(ctx,"SELECT source_id,ingress_seq,record_id,canonical FROM source_ingress_receipts")
 if err!=nil{return err}
 var indexCount int
 for indices.Next(){
  var source string
  var seq int64
  var id,raw []byte
  if err:=indices.Scan(&source,&seq,&id,&raw);err!=nil{indices.Close();return err}
  if seq<0{indices.Close();return errors.New("negative ingress projection sequence")}
  expectedRecord,ok:=canonical[fmt.Sprintf("%s/%d",source,seq)]
  if !ok||!bytes.Equal(expectedRecord.record[:],id)||!bytes.Equal(expectedRecord.raw,raw){
   indices.Close();return errors.New("ingress receipt index differs from canonical record")
  }
  indexCount++
 }
 if err:=indices.Err();err!=nil{indices.Close();return err}
 if err:=indices.Close();err!=nil{return err}
 if indexCount!=len(canonical){return errors.New("missing ingress receipt projection")}
 cursors,err:=q.QueryContext(ctx,"SELECT source_id,next_seq FROM input_source_cursors")
 if err!=nil{return err}
 seen:=map[string]bool{}
 for cursors.Next(){
  var source string
  var next int64
  if err:=cursors.Scan(&source,&next);err!=nil{cursors.Close();return err}
  if next<0||!declared[source]||uint64(next)!=expected[source]{cursors.Close();return errors.New("ingress cursor differs from canonical receipt prefix")}
  seen[source]=true
 }
 if err:=cursors.Err();err!=nil{cursors.Close();return err}
 if err:=cursors.Close();err!=nil{return err}
 for source,count:=range expected{
  if count>0&&!seen[source]{return errors.New("ingress cursor missing for persisted source receipts")}
 }
 return nil
}

// Validates the incoming record and receipt together with source membership
// and closure lifecycle inside the same transaction as publication.
func validateCausalIngress(ctx context.Context,q ingressQuery,source string,seq uint64,record,receipt ports.CanonicalRecord,entry ingressguard.Entry)error{
 rawAny,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{return err}
 f,ok:=rawAny.([]any);if !ok{return errors.New("non-array source ingress")}
 var kind uint64
 var digest domain.Digest
 var derived domain.Digest
 switch len(f){
 case 13:
  kind=1
  derived,err=registeredDigest("cote.csf.id.exogenous-input","cote.csf.schema.exogenous-input-id-preimage",[]any{f[1],f[2],f[3]})
  if err!=nil{return err}
  digest,err=registeredDigest("cote.csf.digest.exogenous-input-unit","cote.csf.schema.exogenous-input.body",f[:12])
  if err!=nil{return err}
  if !equalBytes(f[12],digest[:]){return errors.New("source input unit digest mismatch")}
 case 7:
  kind=2
  derived,err=registeredDigest("cote.csf.id.source-closure","cote.csf.schema.source-closure-id-preimage",[]any{f[1],f[2],f[3]})
  if err!=nil{return err}
  digest,err=registeredDigest("cote.csf.digest.source-closure","cote.csf.schema.source-closure",f)
  if err!=nil{return err}
  if f[6]!=seq{return errors.New("source closure ingress sequence mismatch")}
 default:return errors.New("unsupported source ingress kind")
 }
 if !bytes.Equal(derived[:],record.Identity[:]){return errors.New("source ingress identity invalid")}
 envelope,err:=codec.StrictDecodeEnvelope(receipt.Canonical);if err!=nil{return err}
 if envelope.DomainTag!="cote.csf.record.source-ingress-receipt"||envelope.SchemaID!="cote.csf.schema.source-ingress-receipt"||envelope.SchemaVersion!=1{
  return errors.New("source ingress receipt schema mismatch")
 }
 h:=identity.OpaqueContentHash(receipt.Canonical)
 if !bytes.Equal(h[:],receipt.Identity[:]){return errors.New("source ingress receipt identity invalid")}
 values,ok:=envelope.Value.([]any);if !ok||len(values)!=3||values[0]!=source||values[1]!=seq{return errors.New("source ingress receipt source/sequence mismatch")}
 ref,ok:=values[2].([]any)
 if !ok||len(ref)!=3||ref[0]!=kind||!equalBytes(ref[1],record.Identity[:])||!equalBytes(ref[2],digest[:]){
  return errors.New("source ingress receipt does not authenticate record and digest")
 }
 rows,err:=q.QueryContext(ctx,"SELECT id,canonical FROM input_ledger ORDER BY rowid")
 if err!=nil{return err}
 declared:=false
 hasClosure:=false
 final:=false
 var last domain.EligibilityCoordinate
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return err}
  v,err:=codec.StrictDecodePayload(raw)
  if err!=nil{continue}
  values,ok:=v.([]any)
  if !ok||len(values)<3{continue}
  if len(values)==3&&values[0]=="source-declaration-v1"&&equalBytes(values[1],entry.RunID[:])&&values[2]==source{
   h:=identity.OpaqueContentHash(raw)
   if !bytes.Equal(h[:],id){rows.Close();return errors.New("source declaration lacks canonical identity")}
   declared=true
  }
  if len(values)!=7||!equalBytes(values[1],entry.RunID[:])||values[2]!=source{continue}
  // Projection and replay validate every receipt. Here the causal source
  // lifecycle is folded in durable append order.
  previous,err:=ingressguard.Parse(ports.CanonicalRecord{Identity:bytesToID(id),Canonical:raw},source)
  if err!=nil{rows.Close();return err}
  if final{rows.Close();return errors.New("stored source closure after final")}
  if hasClosure&&previous.Coordinate.Compare(last)<0{rows.Close();return errors.New("stored source closure regressed")}
  last=previous.Coordinate
  hasClosure=true
  value,ok:=values[5].(bool);if !ok{rows.Close();return errors.New("stored source final flag malformed")}
  final=value
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 if !declared{return errors.New("ingress source not declared for run")}
 if final{return errors.New("source ingress after final closure")}
 if hasClosure{
  delta:=entry.Coordinate.Compare(last)
  if kind==1&&delta<=0{return errors.New("source input was not normalized beyond prior closure")}
  if kind==2&&delta<0{return errors.New("source closure coverage regressed")}
 }
 return nil
}

func bytesToID(v []byte)[32]byte{var x [32]byte;copy(x[:],v);return x}
