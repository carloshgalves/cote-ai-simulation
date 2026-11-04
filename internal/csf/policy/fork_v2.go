package policy

import (
 "fmt"
 "os"
 "path/filepath"
)

// validateForkV2Contract converts the signed V2 migration declarations into
// mandatory runtime constraints. A descriptor with a valid SHA256 but missing
// any required safety clause must never be considered an executable bundle.
func validateForkV2Contract(directory string,b *Bundle) error {
 if b==nil{return fmt.Errorf("nil V2 bundle")}
 profile,ok:=b.Profile["fork_migration_v2"].(map[string]any)
 if !ok || profile["policy_id"]!="cote.csf.migration.fork.v2" ||
  profile["origin_codec_policy_id"]!=CodecPolicyID ||
  profile["child_codec_policy_id"]!=CodecPolicyV2ID ||
  profile["history_reference"]!="csfhref-v1" ||
  profile["birth_authority"]!="verified-child-genesis" ||
  profile["inherited_parent_ledgers"]!="archive-only" ||
  profile["atomicity"]!="all-or-nothing" ||
  profile["unsupported_work"]!="reject" {
  return fmt.Errorf("V2 migration profile lacks required replay/atomicity authority")
 }
 transition,ok:=b.Transitions["fork_migration_v2"].(map[string]any)
 if !ok||transition["version"]!=float64(2)||
  transition["origin_policy"]!=CodecPolicyID||
  transition["child_policy"]!=CodecPolicyV2ID||
  transition["history_reference"]!="csfhref-v1"||
  transition["child_causal_origin"]!="fork-history-ref-plus-child-genesis"||
  transition["rekey_parent_id_prohibited"]!=true||
  transition["claim_authority"]!="child-genesis-migration-birth"||
  transition["terminal_delivery_provenance"]!="child-migration-record-authenticated-against-archived-parent-send"||
  transition["atomic_boundary"]!="sqlite-child-bootstrap"||
  transition["unsupported_pending_policy"]!="reject"||
  transition["rollback"]!="rollback-every-child-authority" {
  return fmt.Errorf("V2 migration transition authority incomplete")
 }
 wanted:=[]string{"parent-sent-commit-proof","claim-rekey","transmission-rekey","due-occurrence-rekey","child-lineage-birth"}
 order,ok:=transition["pending_communication_order"].([]any)
 if !ok||len(order)!=len(wanted){return fmt.Errorf("V2 migration order incomplete")}
 for i,x:=range wanted{if order[i]!=x{return fmt.Errorf("V2 migration causal order invalid at %d",i)}}
 bytes,err:=os.ReadFile(filepath.Join(directory,"fork-v2-vectors.json"))
 if err!=nil{return err}
 var fixtures struct{
  Suite string `json:"suite_id"`
  Version uint64 `json:"version"`
  Positive []string `json:"positive"`
  Negative []string `json:"negative"`
 }
 if err:=decodeStrict(bytes,&fixtures);err!=nil{return err}
 if fixtures.Suite!="cote.csf.migration.fork.v2"||fixtures.Version!=2||
  len(fixtures.Positive)!=4||len(fixtures.Negative)!=7{
  return fmt.Errorf("V2 signed migration vectors incomplete")
 }
 return nil
}
