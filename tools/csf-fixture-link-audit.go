// Command csf-fixture-link-audit verifies semantic record-to-reference fixtures.
// It deliberately does not use the independent JavaScript implementation.
package main

import (
    "bytes"
    "crypto/sha256"
    "encoding/binary"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "os"
    "reflect"
)

type fixture struct {
    CaseID string `json:"case_id"`
    Kind string `json:"kind"`
    PersistedRoot string `json:"persisted_root"`
    RecordHex string `json:"record_envelope_hex"`
    IDOperation string `json:"id_operation"`
    IDPreimageHex string `json:"id_preimage_envelope_hex"`
    DigestOperation string `json:"digest_operation"`
    DigestPreimageHex string `json:"digest_preimage_envelope_hex"`
    ExpectedRefHex string `json:"expected_causal_ref_cbor_hex"`
    ExpectedInputRefHex string `json:"expected_input_ref_cbor_hex"`
    AuthoritativeAdmittedUnitHex string `json:"authoritative_admitted_unit_cbor_hex"`
    ReferencedRecordCaseID string `json:"referenced_record_case_id"`
    Candidates []string `json:"candidate_record_envelopes_hex"`
}
type doc struct {
    Semantic []fixture `json:"semantic"`
}
type descriptor struct {
    Operation string `json:"operation"`
    PreimageSchema string `json:"preimage_schema"`
}
type registry struct {
    Identities []struct {
        Code uint64 `json:"code"`
        PersistedRoot string `json:"persisted_root"`
        ID descriptor `json:"id"`
        Digest descriptor `json:"digest"`
    } `json:"reference_identities"`
}

func readUint(b []byte, pos *int, additional byte) (uint64,error) {
    if additional < 24 { return uint64(additional),nil }
    width:=0
    switch additional { case 24: width=1; case 25:width=2; case 26:width=4; case 27:width=8; default:return 0,fmt.Errorf("CBOR additional %d unsupported",additional) }
    if len(b)-*pos < width { return 0,fmt.Errorf("truncated CBOR uint") }
    var result uint64
    for i:=0;i<width;i++ { result=(result<<8)|uint64(b[*pos]);*pos++ }
    return result,nil
}
func decodeItem(b []byte,pos *int) (any,error) {
    if *pos>=len(b) {return nil,fmt.Errorf("truncated CBOR")}
    first:=b[*pos]; *pos++
    major,add:=first>>5,first&31
    if major==7 {
        if add==20{return false,nil};if add==21{return true,nil};if add==22{return nil,nil}
        return nil,fmt.Errorf("unsupported CBOR simple value")
    }
    n,err:=readUint(b,pos,add);if err!=nil{return nil,err}
    switch major {
    case 0:return n,nil
    case 1:if n>uint64(^uint64(0)>>1){return nil,fmt.Errorf("negative CBOR out of range")};return -int64(n)-1,nil
    case 2,3:
        if n>uint64(len(b)-*pos){return nil,fmt.Errorf("truncated bytes/text")}
        raw:=bytes.Clone(b[*pos:*pos+int(n)]);*pos+=int(n)
        if major==2{return raw,nil};return string(raw),nil
    case 4:
        if n>65535{return nil,fmt.Errorf("fixture CBOR array too large")}
        out:=make([]any,0,int(n))
        for i:=uint64(0);i<n;i++{v,err:=decodeItem(b,pos);if err!=nil{return nil,err};out=append(out,v)}
        return out,nil
    default:return nil,fmt.Errorf("CBOR major %d unsupported",major)
    }
}
func decodeBytes(b []byte) (any,error) {
    p:=0;result,err:=decodeItem(b,&p);if err!=nil{return nil,err}
    if p!=len(b){return nil,fmt.Errorf("CBOR trailing bytes")}
    out,err:=encode(result);if err!=nil{return nil,err}
    if !bytes.Equal(out,b){return nil,fmt.Errorf("CBOR not canonical")}
    return result,nil
}
func decodeHex(s string) (any,error){ b,err:=hex.DecodeString(s);if err!=nil{return nil,err};return decodeBytes(b)}
func writeHead(dst *bytes.Buffer, major byte, n uint64) {
    switch {
    case n<24:dst.WriteByte((major<<5)|byte(n))
    case n<=255:dst.WriteByte((major<<5)|24);dst.WriteByte(byte(n))
    case n<=65535:dst.WriteByte((major<<5)|25);var b [2]byte;binary.BigEndian.PutUint16(b[:],uint16(n));dst.Write(b[:])
    case n<=4294967295:dst.WriteByte((major<<5)|26);var b [4]byte;binary.BigEndian.PutUint32(b[:],uint32(n));dst.Write(b[:])
    default:dst.WriteByte((major<<5)|27);var b [8]byte;binary.BigEndian.PutUint64(b[:],n);dst.Write(b[:])
    }
}
func encodeOne(dst *bytes.Buffer,value any) error {
    switch v:=value.(type){
    case uint64:writeHead(dst,0,v)
    case int64:if v>=0{writeHead(dst,0,uint64(v))}else{writeHead(dst,1,uint64(-1-v))}
    case int:if v>=0{writeHead(dst,0,uint64(v))}else{writeHead(dst,1,uint64(-1-v))}
    case []byte:writeHead(dst,2,uint64(len(v)));dst.Write(v)
    case string:writeHead(dst,3,uint64(len(v)));dst.WriteString(v)
    case []any:writeHead(dst,4,uint64(len(v)));for _,item:=range v{if err:=encodeOne(dst,item);err!=nil{return err}}
    case bool:if v{dst.WriteByte(0xf5)}else{dst.WriteByte(0xf4)}
    case nil:dst.WriteByte(0xf6)
    default:return fmt.Errorf("unsupported fixture CBOR type %T",v)
    }
    return nil
}
func encode(v any) ([]byte,error) {var b bytes.Buffer;if err:=encodeOne(&b,v);err!=nil{return nil,err};return b.Bytes(),nil}
func eq(a,b any) bool{return reflect.DeepEqual(a,b)}
func envelope(domain,schema string,payload any) ([]byte,error){
    return encode([]any{[]byte{'C','S','F',0},uint64(1),domain,schema,uint64(1),payload})
}
func digestFor(domain,schema string,payload any) ([32]byte,error){
    b,err:=envelope(domain,schema,payload);if err!=nil{return [32]byte{},err};return sha256.Sum256(b),nil
}
func asFields(v any) ([]any,error){fields,ok:=v.([]any);if !ok{return nil,fmt.Errorf("CBOR fields expected, got %T",v)};return fields,nil}
func indexes(r []any,list ...int) []any {out:=make([]any,len(list));for i,n:=range list{out[i]=r[n]};return out}
func projected(root string,r,prior []any,context any) ([]any,error){
    switch root {
    case "event":return indexes(r,1,7,2),nil
    case "exogenous-input","source-closure":return indexes(r,1,2,3),nil
    case "round-declaration":return indexes(r,6,7,8),nil
    case "slot-dispatch":return indexes(r,1,2,3,4,5),nil
    case "action-proposal":return []any{r[1],uint64(0)},nil
    case "no-proposal":return []any{r[1],uint64(1)},nil
    case "scheduled-occurrence":return indexes(r,9,10,11),nil
    case "trigger-activation":
        origin,err:=asFields(r[5]);if err!=nil{return nil,err}
        rev:=any(uint64(0));if origin[0]==uint64(0){rev=origin[2]}
        return []any{r[1],r[3],r[4],rev,r[7]},nil
    case "commit-candidate":return indexes(r,1,2,3,4,5,6,7),nil
    case "conflict-set":return []any{context,r[1],r[2],r[3]},nil
    case "decision-record":
        subject,err:=asFields(r[4]);if err!=nil{return nil,err}
        return []any{r[2],r[3],subject[2]},nil
    case "admission-fence","cycle-commit","cycle-abort":return indexes(r,0,1,2),nil
    case "attempt-retry":return indexes(r,0,1,2,3),nil
    case "perception-task":return indexes(r,1,3,6,7,8),nil
    case "observation":return indexes(r,1,2,3,4),nil
    case "knowledge-input":
        obs,err:=asFields(r[7]);if err!=nil{return nil,err}
        return []any{obs[1],r[3],r[5],r[1],r[2]},nil
    case "claim","transmission":return indexes(r,1,2,3),nil
    case "snapshot":return indexes(r,0,1,3),nil
    case "genesis-manifest":return indexes(r,0),nil
    case "epistemic-checkpoint-ref":return indexes(r,0,1,2,7),nil
    case "rng-draw":return indexes(r,1,2,3,4,5,6,7,8),nil
    case "affordance-assessment":return indexes(r,0,2),nil
    case "provisional-disposition":return indexes(r,1,2,3,4),nil
    case "attempt-failure":return indexes(r,1,2,3,5,6,4),nil
    default:return nil,fmt.Errorf("unrecognized identity root %s",root)
    }
}
func includes(name string,items []string) bool{for _,x:=range items{if x==name{return true}};return false}
var idStored=[]string{"event","exogenous-input","source-closure","round-declaration","slot-dispatch","action-proposal","scheduled-occurrence","trigger-activation","commit-candidate","conflict-set","decision-record","perception-task","observation","knowledge-input","claim","transmission","no-proposal","rng-draw","provisional-disposition","attempt-failure"}
var bodyDigest=[]string{"event","exogenous-input","action-proposal","scheduled-occurrence","trigger-activation","commit-candidate","conflict-set","decision-record","admission-fence","cycle-abort","observation","knowledge-input","claim","transmission","snapshot","no-proposal","rng-draw","affordance-assessment","provisional-disposition","attempt-failure"}
func conflictCycle(item fixture, conflict []any)(any,error){
    if len(item.Candidates)!=2{return nil,fmt.Errorf("conflict requires two candidate witnesses")}
    var shared []byte
    for i,raw:=range item.Candidates {
        candidateEnvelope,err:=decodeHex(raw);if err!=nil{return nil,err}
        c,err:=asFields(candidateEnvelope);if err!=nil{return nil,err}
        if c[3]!="cote.csf.schema.commit-candidate"{return nil,fmt.Errorf("candidate wrong schema")}
        f,err:=asFields(c[5]);if err!=nil{return nil,err}
        calcID,err:=digestFor("cote.csf.id.commit-candidate","cote.csf.schema.candidate-id-preimage",indexes(f,1,2,3,4,5,6,7));if err!=nil{return nil,err}
        if !bytes.Equal(f[0].([]byte),calcID[:]){return nil,fmt.Errorf("candidate ID not derived")}
        calcDigest,err:=digestFor("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",f[:len(f)-1]);if err!=nil{return nil,err}
        if !bytes.Equal(f[len(f)-1].([]byte),calcDigest[:]){return nil,fmt.Errorf("candidate body digest mismatch")}
        if i==0{shared=f[2].([]byte)}else if !bytes.Equal(shared,f[2].([]byte)){return nil,fmt.Errorf("candidates from different cycles")}
        ids,err:=asFields(conflict[1]);if err!=nil{return nil,err}
        if !bytes.Equal(ids[i].([]byte),calcID[:]){return nil,fmt.Errorf("conflict candidate IDs differ from witnesses")}
    }
    return shared,nil
}
func checkReference(f fixture,registries registry) error{
    env,err:=decodeHex(f.RecordHex);if err!=nil{return err}
    outer,err:=asFields(env);if err!=nil{return err}
    if outer[3]!=f.PersistedRoot{return fmt.Errorf("record schema mismatch")}
    r,err:=asFields(outer[5]);if err!=nil{return err}
    root:=f.PersistedRoot[len("cote.csf.schema."):]
    var target *struct {Code uint64 `json:"code"`;PersistedRoot string `json:"persisted_root"`;ID descriptor `json:"id"`;Digest descriptor `json:"digest"`}
    _ = target // actual registry comparison below is field-by-field
    found:=false
    var expectedCode uint64
    var idOp,idSchema,digOp,digSchema string
    for _,entry:=range registries.Identities{if entry.PersistedRoot==f.PersistedRoot{
        found=true;expectedCode=entry.Code;idOp=entry.ID.Operation;idSchema=entry.ID.PreimageSchema;digOp=entry.Digest.Operation;digSchema=entry.Digest.PreimageSchema;break
    }}
    if !found||idOp!=f.IDOperation||digOp!=f.DigestOperation{return fmt.Errorf("registry mismatch")}
    idRaw,err:=hex.DecodeString(f.IDPreimageHex);if err!=nil{return err}
    dRaw,err:=hex.DecodeString(f.DigestPreimageHex);if err!=nil{return err}
    idEnvelope,err:=decodeBytes(idRaw);if err!=nil{return err}
    digestEnvelope,err:=decodeBytes(dRaw);if err!=nil{return err}
    idParts,err:=asFields(idEnvelope);if err!=nil{return err}
    digParts,err:=asFields(digestEnvelope);if err!=nil{return err}
    if idParts[2]!=idOp||idParts[3]!=idSchema||digParts[2]!=digOp||digParts[3]!=digSchema{return fmt.Errorf("unregistered preimage operation")}
    old,err:=asFields(idParts[5]);if err!=nil{return err}
    var context any
    if root=="conflict-set" {context,err=conflictCycle(f,r);if err!=nil{return err}}
    expected,err:=projected(root,r,old,context);if err!=nil{return err}
    if !eq(expected,old){return fmt.Errorf("ID preimage disconnected from record")}
    var body any
    if root=="cycle-commit"{
        parts,err:=asFields(digParts[5]);if err!=nil{return err}
        events,err:=asFields(parts[1]);if err!=nil{return err}
        expectedEventIDs,err:=asFields(r[15]);if err!=nil{return err}
        if len(events)!=len(expectedEventIDs) ||len(events)>0{return fmt.Errorf("cycle commit event witness missing")}
        body=[]any{r[:len(r)-1],events}
    }else if includes(root,bodyDigest){body=r[:len(r)-1]}else{body=r}
    if !eq(digParts[5],body){return fmt.Errorf("digest preimage disconnected from record")}
    computedID:=sha256.Sum256(idRaw)
    computedDigest:=sha256.Sum256(dRaw)
    if includes(root,idStored)&&!bytes.Equal(r[0].([]byte),computedID[:]){return fmt.Errorf("record ID does not match identity operation")}
    if includes(root,bodyDigest)&&!bytes.Equal(r[len(r)-1].([]byte),computedDigest[:]){return fmt.Errorf("record digest does not match body")}
    refValue,err:=decodeHex(f.ExpectedRefHex);if err!=nil{return err}
    ref,err:=asFields(refValue);if err!=nil{return err}
    if len(ref)!=3||ref[0]!=expectedCode||!bytes.Equal(ref[1].([]byte),computedID[:])||!bytes.Equal(ref[2].([]byte),computedDigest[:]){return fmt.Errorf("causal reference mismatch")}
    if f.ExpectedInputRefHex!=""{
        encoded,err:=decodeHex(f.ExpectedInputRefHex);if err!=nil{return err}
        inputRef,err:=asFields(encoded);if err!=nil{return err}
        if !bytes.Equal(inputRef[len(inputRef)-2].([]byte),computedID[:])||!bytes.Equal(inputRef[len(inputRef)-1].([]byte),computedDigest[:]){return fmt.Errorf("input reference mismatch")}
    }
    return nil
}
func run()error{
    constDir:="docs/architecture/canonical-codec-v1-bundle/"
    raw,err:=os.ReadFile(constDir+"fixtures.json");if err!=nil{return err}
    var f doc;if err=json.Unmarshal(raw,&f);err!=nil{return err}
    raw,err=os.ReadFile(constDir+"registries.json");if err!=nil{return err}
    var r registry;if err=json.Unmarshal(raw,&r);err!=nil{return err}
    byID:=map[string]fixture{}
    count:=0;roots:=map[string]bool{}
    for _,item:=range f.Semantic {
        if item.Kind!="record_to_reference"{continue}
        count++;roots[item.PersistedRoot]=true;byID[item.CaseID]=item
        if err:=checkReference(item,r);err!=nil{return fmt.Errorf("%s: %w",item.CaseID,err)}
    }
    if count!=28||len(roots)!=28{return fmt.Errorf("expected 28 distinct reference roots, got %d/%d",count,len(roots))}
    x,ok:=byID["reference.record-to-ref.exogenous-input"];if !ok{return fmt.Errorf("missing exogenous source")}
    ref,err:=decodeHex(x.ExpectedInputRefHex);if err!=nil{return err}
    fields,err:=asFields(ref);if err!=nil{return err}
    admitted:=0
    for _,item:=range f.Semantic{
        if item.Kind!="admitted_unit_ledger_mismatch"{continue}
        admitted++
        if item.ReferencedRecordCaseID!=x.CaseID{return fmt.Errorf("admitted source wrong")}
        value,err:=decodeHex(item.AuthoritativeAdmittedUnitHex);if err!=nil{return err}
        v,err:=asFields(value);if err!=nil{return err}
        if !bytes.Equal(v[3].([]byte),fields[len(fields)-2].([]byte))||!bytes.Equal(v[4].([]byte),fields[len(fields)-1].([]byte)){return fmt.Errorf("admitted authority ID/digest mismatch")}
    }
    if admitted!=5{return fmt.Errorf("expected five admitted cases, got %d",admitted)}
    fmt.Printf("Independent Go verification: %d/28 record references, %d admitted authorities, ConflictSet candidates proven\n",count,admitted)
    return nil
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
