// Package tgdata exposes a read-only, owner-facing projection of the consolidated
// Tomodachi Game character corpus. It is not an agent knowledge source.
package tgdata

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed consolidated/characters/*.json
var corpus embed.FS

type Domain struct {
	Description string   `json:"description,omitempty"`
	DescriptionStatus string `json:"description_status,omitempty"`
	BehavioralCaseRefs []string `json:"behavioral_case_refs,omitempty"`
	ContextEvidenceRefs []string `json:"context_evidence_refs,omitempty"`
}

type Pack struct {
	SchemaVersion string `json:"schema_version"`
	ID string `json:"character_id"`
	Name string `json:"display_name"`
	Tier string `json:"modeling_tier"`
	SourceStatus string `json:"source_status"`
	SourceCoverage []string `json:"source_coverage"`
	Identity struct {
		RoleSummary string `json:"role_summary"`
		Aliases []string `json:"aliases"`
	} `json:"identity"`
	Domains map[string]Domain `json:"domains"`
	SourceDocuments []string `json:"source_documents"`
	HistoricalContextOnly bool `json:"historical_context_only"`
	Uncertainties []string `json:"uncertainties"`
	KnowledgeTimePolicy map[string]bool `json:"knowledge_time_policy"`
	RetrievalPolicy map[string]string `json:"retrieval_policy"`
	FidelityEval struct { AntiCase string `json:"anti_case"` } `json:"fidelity_eval"`
}

// Load indexes embedded packs by their actual IDs, never by request paths.
func Load() (map[string]Pack, error) {
	files, err := fs.Glob(corpus, "consolidated/characters/*.json")
	if err != nil { return nil, err }
	sort.Strings(files)
	if len(files)==0 {return nil, fmt.Errorf("empty Tomodachi corpus")}
	out:=make(map[string]Pack,len(files))
	for _, file:=range files {
		raw,err:=corpus.ReadFile(file);if err!=nil{return nil,err}
		var p Pack
		if err=json.Unmarshal(raw,&p);err!=nil{return nil,fmt.Errorf("%s: %w",file,err)}
		if !strings.HasPrefix(p.ID,"actor.")||p.Name==""||p.SchemaVersion!="consolidated-character-v2"||p.Tier=="" {return nil,fmt.Errorf("invalid character pack: %s",file)}
		if _,ok:=out[p.ID];ok{return nil,fmt.Errorf("duplicate character ID %s",p.ID)}
		out[p.ID]=p
	}
	return out,nil
}
