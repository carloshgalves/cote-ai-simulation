// Package schedule owns immutable pending occurrence identities and deterministic
// minimum-eligibility selection. Terminal lifecycle belongs to CSFRV1-5.
package schedule

import (
 "errors"
 "sort"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

const (
 Pending uint8=iota
 Cancelled
 Consumed
)

type Occurrence struct {
 ID domain.ID
 RunID domain.ID
 SourceID string
 DueAt domain.SimulationInstant
 Eligibility domain.EligibilityCoordinate
 Lifecycle uint8
 Kind string
 Payload []byte
}

// PendingRecords never gives arrival/order-of-registration a causal priority.
// Duplicate IDs are corrupt even if the payload happens to agree.
func PendingRecords(in []Occurrence) ([]clock.Pending,error) {
 seen:=map[domain.ID]struct{}{}
 out:=make([]clock.Pending,0,len(in))
 for _,o:=range in {
  if o.SourceID=="" || o.Kind=="" || o.DueAt>o.Eligibility.NotBeforeInstant{return nil,errors.New("invalid scheduled occurrence")}
  if o.Lifecycle!=Pending && o.Lifecycle!=Cancelled && o.Lifecycle!=Consumed{return nil,errors.New("invalid lifecycle")}
  if _,ok:=seen[o.ID];ok{return nil,errors.New("duplicate scheduled occurrence")}
  seen[o.ID]=struct{}{}
  if o.Lifecycle==Pending {out=append(out,clock.Pending{ID:o.ID,Eligibility:o.Eligibility})}
 }
 sort.Slice(out,func(i,j int)bool {
  a,b:=out[i],out[j];if d:=a.Eligibility.Compare(b.Eligibility);d!=0{return d<0}
  for k:=0;k<len(a.ID);k++{if a.ID[k]!=b.ID[k]{return a.ID[k]<b.ID[k]}}
  return false
 })
 return out,nil
}
