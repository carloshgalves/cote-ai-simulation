package fork

import (
 "bytes"
 "encoding/hex"
 "testing"
)

const normativeReferenceHex = "435346485245460001001a636f74652e6373662e636f6465632e63626f722d6465742e76310020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f0016636f74652e6373662e686173682e736e617073686f740018636f74652e6373662e736368656d612e736e617073686f74000000000000000100077368612d3235360020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func TestCSFRV18NormativeCrossPolicyHistoryReferenceVector(t *testing.T){
 source,err:=hex.DecodeString(normativeReferenceHex);if err!=nil{t.Fatal(err)}
 decoded,err:=Decode(source);if err!=nil{t.Fatal(err)}
 if decoded.CheckpointSchemaVersion!=1||decoded.DigestAlgorithmID!=Algorithm||
  decoded.CheckpointDomainTag!=SnapshotDomain||decoded.CheckpointSchemaID!=SnapshotSchema{
  t.Fatal("history ref fields changed across decoding")
 }
 for i:=0;i<32;i++ {
  if decoded.OriginPolicyHash[i]!=byte(i)||decoded.CheckpointDigest[i]!=byte(i){
   t.Fatal("cross-policy reference corrupted the original hashes")
  }
 }
 encoded,err:=Encode(decoded);if err!=nil{t.Fatal(err)}
 if !bytes.Equal(source,encoded){t.Fatalf("normative V1 bytes differ:\nactual %x\nexpected %x",encoded,source)}
}

func TestCSFRV18CrossPolicyHistoryRefRejectsMalformed(t *testing.T){
 ref,err:=hex.DecodeString(normativeReferenceHex);if err!=nil{t.Fatal(err)}
 cases:=map[string][]byte{
  "missing":{},
  "wrong-magic":append([]byte(nil),ref...),
  "wrong-version":append([]byte(nil),ref...),
  "short":ref[:len(ref)-1],
  "trailing":append(append([]byte(nil),ref...),byte(0)),
  "zero-policy-length":append([]byte(nil),ref...),
  "wrong-policy-hash-length":append([]byte(nil),ref...),
  "wrong-digest-length":append([]byte(nil),ref...),
  "unsupported-digest-algorithm":append([]byte(nil),ref...),
 }
 cases["wrong-magic"][0]=1
 cases["wrong-version"][8]=2
 cases["zero-policy-length"][9],cases["zero-policy-length"][10]=0,0
 // Policy id occupies 26 bytes following 9-byte fixed header and u16 size.
 cases["wrong-policy-hash-length"][37]=0
 cases["wrong-policy-hash-length"][38]=31
 // Last 34 bytes are the digest length prefix + SHA256 digest.
 cases["wrong-digest-length"][len(ref)-34]=0
 cases["wrong-digest-length"][len(ref)-33]=31
 // Locate SHA256 algorithm string without touching any other bytes.
 index:=bytes.Index(cases["unsupported-digest-algorithm"],[]byte("sha-256"))
 if index<0{t.Fatal("missing algorithm in normative vector")}
 cases["unsupported-digest-algorithm"][index]='x'
 for name,raw:=range cases{
  t.Run(name,func(t *testing.T){
   if _,err:=Decode(raw);err==nil{t.Fatal("accepted malformed or unsupported fork reference")}
  })
 }
}
