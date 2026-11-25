package pilot

import (
	_ "embed"
	"encoding/json"
)

//go:embed canon_projection.json
var canonProjectionJSON []byte

type CanonProjection struct {
	Source string `json:"source"`
	ClassRef string `json:"class_ref"`
	SchoolYear int `json:"school_year"`
	PrivateKnowledge []string `json:"private_knowledge"`
	Dispositions []string `json:"dispositions"`
	Status string `json:"status"`
}
func canonFor(actor string) CanonProjection {
	var projections map[string]CanonProjection
	if err:=json.Unmarshal(canonProjectionJSON,&projections);err!=nil{panic(err)}
	return projections[actor]
}
