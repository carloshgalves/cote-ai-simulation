package pilot

import (
 "crypto/sha256"
 "fmt"
)

// Limits only discretionary opportunities. An addressed conversation,
// active examination, invitation, or other real stimulus is never discarded.
func dailyOpportunityBudget(m Mind)int{
 if m.Tier==FullTier{return 3}
 return 1
}
func essentialCognition(j Journal,s State,id string)bool{
 m:=s.Minds[id]
 if m.Conversation!=nil&&m.Conversation.Turn==id{return true}
 if m.PendingStimulus!=""{return true}
 if s.World!=nil{
  if e:=s.World.Exam;e!=nil&&has(e.Group,id)&&(e.Phase=="early"||e.Phase=="final")&&!e.Submitted[id]&&!m.AwaitEvent&&(j.Version<CalendarVersion||m.LastDeliberationMinute==0||s.Minute-m.LastDeliberationMinute>=20){return true}
  for _,mt:=range s.World.Meetings{if mt.Guest==id&&mt.Status=="pending"&&s.Minute<mt.At{return true}}
 }
 return false
}
func calibratedNeedsModel(j Journal,s State,id string)bool{
 m:=s.Minds[id]
 if essentialCognition(j,s,id){return true}
 if m.NextDeliberation>0&&s.Minute>=m.NextDeliberation{return true}
 if m.NextDeliberation>s.Minute{return false}
 if j.Version>=HierarchicalVersion{return strategicOpportunity(j,s,id)}
 return personalOpportunity(j,s,id)!=nil
}
func rotatedOpportunityPeers(j Journal,s State,id string,peers []string)[]string{
 if len(peers)==0{return peers}
 h:=sha256.Sum256([]byte(fmt.Sprintf("%s/opportunity/%s/%d/%d",j.WorldSeed,id,s.Minute/1440,mindOpportunityCount(s,id))))
 n:=int(h[0])%len(peers)
 return append(append([]string{},peers[n:]...),peers[:n]...)
}
func mindOpportunityCount(s State,id string)int{return s.Minds[id].OpportunityCount}
