package testresourcev1

import (
 "bytes"
 "testing"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestSyntheticActionRegistryProducesIdenticalDecisionAcrossWorkerOrders(t *testing.T){
 b,err:=policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle");if err!=nil{t.Fatal(err)}
 q,err:=LastUnit(b);if err!=nil{t.Fatal(err)}
 first,err:=decision.Run(q);if err!=nil{t.Fatal(err)}
 if first.PostAvailable!=0||len(first.Conflicts)!=1||len(first.Draws)!=1{t.Fatal("scenario missing contest evidence")}
 q.Units[0],q.Units[1]=q.Units[1],q.Units[0]
 next,err:=decision.Run(q);if err!=nil||!bytes.Equal(first.Canonical,next.Canonical){t.Fatalf("order-sensitive source extension: %v",err)}
}
