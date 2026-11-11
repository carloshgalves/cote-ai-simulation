// Package fork contains the versioned, policy-independent checkpoint reference
// framing from the CSFHREF V1 contract. No child runtime may infer a parent's
// codec/schema from the child's current policy.
package fork

import (
 "bytes"
 "encoding/binary"
 "errors"
 "fmt"
 "io"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

var magic = [8]byte{'C','S','F','H','R','E','F',0}

const (
 ReferenceVersion = 1
 SnapshotDomain = "cote.csf.hash.snapshot"
 SnapshotSchema = "cote.csf.schema.snapshot"
 Algorithm = "sha-256"
)

type HistoryReference struct {
 OriginPolicyID string
 OriginPolicyHash [32]byte
 CheckpointDomainTag string
 CheckpointSchemaID string
 CheckpointSchemaVersion uint64
 DigestAlgorithmID string
 CheckpointDigest [32]byte
}

func (h HistoryReference) Validate()error{
 if err:=domain.ValidateMachineID(h.OriginPolicyID);err!=nil{return fmt.Errorf("origin policy id: %w",err)}
 if err:=domain.ValidateDomainTag(h.CheckpointDomainTag);err!=nil{return fmt.Errorf("checkpoint domain: %w",err)}
 if err:=domain.ValidateDomainTag(h.CheckpointSchemaID);err!=nil{return fmt.Errorf("checkpoint schema: %w",err)}
 if h.CheckpointSchemaVersion==0{return errors.New("checkpoint schema version must be positive")}
 if h.DigestAlgorithmID!=Algorithm{return errors.New("unsupported checkpoint digest algorithm")}
 for _,s:=range []string{h.OriginPolicyID,h.CheckpointDomainTag,h.CheckpointSchemaID,h.DigestAlgorithmID}{
  if len(s)==0||len(s)>65535{return errors.New("invalid history ref text size")}
  for i:=0;i<len(s);i++{if s[i]>127||s[i]==0{return errors.New("history ref requires ASCII text")}}
 }
 return nil
}

// Encode uses exactly the normative CSFHREF framing, without a CBOR/child
// policy wrapper. The canonical reference is not a mutable storage locator.
func Encode(h HistoryReference)([]byte,error){
 if err:=h.Validate();err!=nil{return nil,err}
 var out bytes.Buffer
 out.Write(magic[:])
 out.WriteByte(ReferenceVersion)
 writeText:=func(s string){
  var size [2]byte;binary.BigEndian.PutUint16(size[:],uint16(len(s)));out.Write(size[:]);out.WriteString(s)
 }
 writeHash:=func(hash [32]byte){
  var size [2]byte;binary.BigEndian.PutUint16(size[:],32);out.Write(size[:]);out.Write(hash[:])
 }
 writeText(h.OriginPolicyID)
 writeHash(h.OriginPolicyHash)
 writeText(h.CheckpointDomainTag)
 writeText(h.CheckpointSchemaID)
 var version [8]byte;binary.BigEndian.PutUint64(version[:],h.CheckpointSchemaVersion);out.Write(version[:])
 writeText(h.DigestAlgorithmID)
 writeHash(h.CheckpointDigest)
 return out.Bytes(),nil
}

// Decode rejects truncation, trailing bytes, wrong magic, wrong size and
// unsupported versions *before* handing the ref to a policy resolver.
func Decode(raw []byte)(HistoryReference,error){
 var h HistoryReference
 reader:=bytes.NewReader(raw)
 var actual [8]byte
 if _,err:=io.ReadFull(reader,actual[:]);err!=nil{return h,errors.New("truncated checkpoint ref header")}
 if actual!=magic{return h,errors.New("invalid checkpoint ref magic")}
 version,err:=reader.ReadByte()
 if err!=nil||version!=ReferenceVersion{return h,errors.New("unknown checkpoint ref framing version")}
 readLength:=func() (uint16,error){
  var n [2]byte
  if _,err:=io.ReadFull(reader,n[:]);err!=nil{return 0,errors.New("truncated history ref length")}
  return binary.BigEndian.Uint16(n[:]),nil
 }
 readString:=func()(string,error){
  n,err:=readLength();if err!=nil{return "",err}
  if n==0||int(n)>reader.Len(){return "",errors.New("truncated or empty history ref string")}
  data:=make([]byte,n);if _,err:=io.ReadFull(reader,data);err!=nil{return "",err}
  for _,v:=range data{if v==0||v>127{return "",errors.New("history ref string is not ASCII")}}
  return string(data),nil
 }
 readHash:=func()([32]byte,error){
  var result [32]byte
  n,err:=readLength();if err!=nil{return result,err}
  if n!=32{return result,errors.New("history ref SHA256 length must be 32")}
  if _,err:=io.ReadFull(reader,result[:]);err!=nil{return result,errors.New("truncated history ref hash")}
  return result,nil
 }
 h.OriginPolicyID,err=readString();if err!=nil{return h,err}
 h.OriginPolicyHash,err=readHash();if err!=nil{return h,err}
 h.CheckpointDomainTag,err=readString();if err!=nil{return h,err}
 h.CheckpointSchemaID,err=readString();if err!=nil{return h,err}
 var ver [8]byte;if _,err:=io.ReadFull(reader,ver[:]);err!=nil{return h,errors.New("truncated history ref schema version")}
 h.CheckpointSchemaVersion=binary.BigEndian.Uint64(ver[:])
 h.DigestAlgorithmID,err=readString();if err!=nil{return h,err}
 h.CheckpointDigest,err=readHash();if err!=nil{return h,err}
 if reader.Len()!=0{return h,errors.New("trailing bytes in checkpoint ref")}
 if err:=h.Validate();err!=nil{return h,err}
 canonical,err:=Encode(h);if err!=nil||!bytes.Equal(canonical,raw){return h,errors.New("noncanonical history ref")}
 return h,nil
}

func NewSnapshotReference(bundle *policy.Bundle,canonical []byte)(HistoryReference,error){
 if bundle==nil||len(canonical)==0{return HistoryReference{},errors.New("missing origin policy or canonical checkpoint")}
 originID,_,originHash,_,err:=bundle.CodecIdentity()
 if err!=nil{return HistoryReference{},fmt.Errorf("unverified origin policy: %w",err)}
 digest:=identity.OpaqueContentHash(canonical)
 h:=HistoryReference{
  OriginPolicyID:originID,
  OriginPolicyHash:originHash,
  CheckpointDomainTag:SnapshotDomain,
  CheckpointSchemaID:SnapshotSchema,
  CheckpointSchemaVersion:1,
  DigestAlgorithmID:Algorithm,
  CheckpointDigest:digest,
 }
 return h,h.Validate()
}

// VerifyOriginSnapshot validates the checkpoint's registered identity under
// the ORIGIN bundle. A caller supplying only a child policy cannot satisfy it.
func VerifyOriginSnapshot(h HistoryReference,canonical []byte,origin *policy.Bundle)error{
 if origin==nil{return errors.New("origin policy bundle not available")}
 if err:=h.Validate();err!=nil{return err}
 installedID,_,installedHash,_,err:=origin.CodecIdentity()
 if err!=nil{return fmt.Errorf("origin policy not verified: %w",err)}
 if h.OriginPolicyID!=installedID||h.OriginPolicyHash!=installedHash||
  h.CheckpointDomainTag!=SnapshotDomain||h.CheckpointSchemaID!=SnapshotSchema||
  h.CheckpointSchemaVersion!=1{
  return errors.New("checkpoint reference origin policy or snapshot schema unsupported")
 }
 payload,err:=codec.StrictDecodePayload(canonical);if err!=nil{return err}
 fields,ok:=payload.([]any)
 if !ok||len(fields)!=19{return errors.New("origin checkpoint schema is not snapshot V1")}
 digest,_,err:=identity.New(origin).Sum(SnapshotDomain,"cote.csf.schema.snapshot.body",1,fields[:18])
 if err!=nil{return err}
 stored,ok:=fields[18].([]byte)
 if !ok||len(stored)!=32||!bytes.Equal(stored,digest[:]){
  return errors.New("referenced checkpoint hash differs from origin policy digest")
 }
 exactBytes:=identity.OpaqueContentHash(canonical)
 if h.CheckpointDigest!=[32]byte(exactBytes){return errors.New("origin checkpoint exact-byte SHA256 mismatch")}
 return nil
}
