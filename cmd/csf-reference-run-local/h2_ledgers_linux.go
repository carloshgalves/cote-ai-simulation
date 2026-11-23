//go:build linux

package main

import (
 "bytes"
 "context"
 "database/sql"
 "encoding/binary"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "net/url"
 "os"
 "path/filepath"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

// H2-L: these private exports are evidence bytes, NEVER an alternative store
// or permission to submit writes/physical cuts. They are not public artifacts.
var ledgerNamesH2 = [...]string{
 "input_ledger", "decision_ledger", "event_store", "evidence_ledger", "causal_outbox",
}
var h2PrivatePaths = []string{
 "ledger-export/input_ledger.bin",
 "ledger-export/decision_ledger.bin",
 "ledger-export/event_store.bin",
 "ledger-export/evidence_ledger.bin",
 "ledger-export/causal_outbox.bin",
 "ledger-export/ledger-proof.json",
 "ledger-export/owner-audit.json",
}

const ledgerMagicH2 = "CSF-H2-LEDGER-V1\x00"
const maxLedgerExportH2 = 128 << 20

type ledgerHeadH2 struct {
 Count int `json:"count"`
 PrefixDigest string `json:"prefix_digest"`
 ExportSHA256 string `json:"export_sha256"`
}
type ledgerProofH2 struct {
 Schema string `json:"schema"`
 SourceSHA string `json:"source_sha"`
 RunID string `json:"run_id"`
 PolicyHash string `json:"policy_hash"`
 Ledgers map[string]ledgerHeadH2 `json:"ledgers"`
 Disposition string `json:"disposition"`
}

// This stream holds the authentic raw canonical bytes in private storage,
// with domain+ID+length framing. The digest is the canonical CSF ledger-prefix
// digest, not SHA256 of this local framing.
func captureLedgerH2(ctx context.Context, db *sql.DB, name string, p *policy.Bundle) ([]byte, ledgerHeadH2, error) {
 var zero ledgerHeadH2
 allowed:=false
 for _,v:=range ledgerNamesH2 { if name==v { allowed=true } }
 if !allowed || p==nil { return nil,zero,errors.New("invalid ledger or policy") }
 rows,err:=db.QueryContext(ctx,"SELECT id, canonical FROM "+name+" ORDER BY id")
 if err!=nil{return nil,zero,err}
 defer rows.Close()
 buf:=bytes.NewBufferString(ledgerMagicH2)
 buf.WriteByte(byte(len(name)))
 buf.WriteString(name)
 var rawRecords []any
 var last []byte
 count:=0
 for rows.Next(){
  var id, canonical []byte
  if err:=rows.Scan(&id,&canonical);err!=nil{return nil,zero,err}
  if len(id)!=32||len(canonical)==0||len(canonical)>4<<20 ||
    (last!=nil && bytes.Compare(last,id)>=0) {
   return nil,zero,errors.New("invalid/duplicate ledger entry")
  }
  if _,err:=codec.StrictDecodePayload(canonical);err!=nil{return nil,zero,err}
  if buf.Len()+32+4+len(canonical)>maxLedgerExportH2{
   return nil,zero,errors.New("private ledger export size ceiling")
  }
  buf.Write(id)
  var length [4]byte
  binary.BigEndian.PutUint32(length[:],uint32(len(canonical)))
  buf.Write(length[:])
  buf.Write(canonical)
  rawRecords=append(rawRecords,bytes.Clone(canonical))
  last=bytes.Clone(id)
  count++
 }
 if err:=rows.Err();err!=nil{return nil,zero,err}
 if count==0{return nil,zero,errors.New("required ledger empty")}
 digest,_,err:=identity.New(p).Sum("cote.csf.digest.ledger-prefix",
  "cote.csf.schema.ledger-prefix-digest-preimage",1,[]any{name,rawRecords})
 if err!=nil{return nil,zero,err}
 raw:=buf.Bytes()
 return raw,ledgerHeadH2{Count:count,PrefixDigest:hex.EncodeToString(digest[:]),
  ExportSHA256:hashBytes(raw)},nil
}

func detachedDBH2(original string)(string,func(),error) {
 dir,err:=os.MkdirTemp("","csf-h2-detached-")
 if err!=nil{return "",nil,err}
 cleanup:=func(){_ = os.RemoveAll(dir)}
 if err:=os.Chmod(dir,0700);err!=nil{cleanup();return "",nil,err}
 dest:=filepath.Join(dir,"causal.db")
 for _,suffix:=range []string{"","-wal","-shm"}{
  bytes,err:=readPrivateRegular(original+suffix)
  if errors.Is(err,os.ErrNotExist)&&suffix!=""{continue}
  if err!=nil{cleanup();return "",nil,err}
  if err:=os.WriteFile(dest+suffix,bytes,0600);err!=nil{cleanup();return "",nil,err}
 }
 return dest,cleanup,nil
}

func readOnlyDBH2(ctx context.Context,path string)(*sql.DB,error){
 abs,err:=filepath.Abs(path)
 if err!=nil{return nil,err}
 uri:=(&url.URL{Scheme:"file",Path:filepath.ToSlash(abs),RawQuery:"mode=ro&_pragma=query_only%28ON%29"}).String()
 db,err:=sql.Open("sqlite",uri)
 if err!=nil{return nil,err}
 db.SetMaxOpenConns(1)
 if err:=db.PingContext(ctx);err!=nil{db.Close();return nil,err}
 var ro int
 if err:=db.QueryRowContext(ctx,"PRAGMA query_only").Scan(&ro);err!=nil||ro!=1{
  db.Close();return nil,errors.New("H2 must query detached db in query-only mode")
 }
 return db,nil
}

func exportH2Ledgers(ctx context.Context,out,sha,run string,p *policy.Bundle)error{
 if p==nil{return errors.New("missing H2 policy")}
 dir:=filepath.Join(out,"ledger-export")
 if err:=os.Mkdir(dir,0700);err!=nil{return err}
 detached,cleanup,err:=detachedDBH2(filepath.Join(out,"causal.db"))
 if err!=nil{return err}
 defer cleanup()
 db,err:=readOnlyDBH2(ctx,detached)
 if err!=nil{return err}
 defer db.Close()
 proof:=ledgerProofH2{Schema:"csfrv1-10h.ledger-evidence.v1",SourceSHA:sha,RunID:run,
  PolicyHash:hex.EncodeToString(p.CodecPolicyHash[:]),
  Ledgers:make(map[string]ledgerHeadH2,5),Disposition:"PRIVATE_LOCAL_ONLY_NO_PRODUCTION_GO"}
 for _,name:=range ledgerNamesH2{
  raw,head,err:=captureLedgerH2(ctx,db,name,p)
  if err!=nil{return fmt.Errorf("H2 export %s: %w",name,err)}
  if err:=exclusiveBytes(filepath.Join(dir,name+".bin"),raw);err!=nil{return err}
  proof.Ledgers[name]=head
 }
 return exclusiveJSON(filepath.Join(dir,"ledger-proof.json"),proof)
}

func verifyH2Ledgers(ctx context.Context,out,sha,run string,p *policy.Bundle)error{
 dir:=filepath.Join(out,"ledger-export")
 originalProof,err:=readPrivateRegular(filepath.Join(dir,"ledger-proof.json"))
 if err!=nil{return err}
 var proof ledgerProofH2
 if err:=json.Unmarshal(originalProof,&proof);err!=nil{return err}
 if !exactLocalJSON(originalProof,proof)||
  proof.Schema!="csfrv1-10h.ledger-evidence.v1" ||
  proof.SourceSHA!=sha || proof.RunID!=run ||
  proof.Disposition!="PRIVATE_LOCAL_ONLY_NO_PRODUCTION_GO" ||
  proof.PolicyHash!=hex.EncodeToString(p.CodecPolicyHash[:])||
  len(proof.Ledgers)!=len(ledgerNamesH2){
  return errors.New("H2 malformed or foreign ledger certificate")
 }
 detached,cleanup,err:=detachedDBH2(filepath.Join(out,"causal.db"))
 if err!=nil{return err}
 defer cleanup()
 db,err:=readOnlyDBH2(ctx,detached)
 if err!=nil{return err}
 defer db.Close()
 for _,name:=range ledgerNamesH2{
  expected,head,err:=captureLedgerH2(ctx,db,name,p)
  if err!=nil{return err}
  archived,err:=readPrivateRegular(filepath.Join(dir,name+".bin"))
  if err!=nil{return err}
  if !bytes.Equal(expected,archived) || head!=proof.Ledgers[name]{
   return fmt.Errorf("H2 detached ledger differs from private export: %s",name)
  }
 }
 return nil
}
