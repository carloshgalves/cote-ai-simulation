// Package tomodachi runs a bounded autonomous Kokkuri experiment over the
// prospective pure Go reducer. This is an owner-side experimental runner;
// it is not the CSF CommitCoordinator or the durable native event store.
package tomodachi

import (
 "context"
 "encoding/hex"
 "errors"
 "fmt"
 "strings"

 game "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer/tomodachi"
)

type Guidance struct {
 Guidance []string `json:"guidance"`
 EvidenceIDs []string `json:"evidence_ids"`
 ManifestDigest string `json:"manifest_digest"`
}
type Retrieve func(context.Context,string,string,string)(Guidance,error)
type Model func(context.Context,Input)(game.Decision,error)
type Input struct{
 Observation game.View `json:"observation"`
 Identity string `json:"identity"`
 BehavioralGuidance []string `json:"behavioral_guidance"`
}
type RetrievalReceipt struct{
 Actor string `json:"actor"`
 EvidenceIDs []string `json:"evidence_ids"`
 ManifestDigest string `json:"manifest_digest"`
}
type Frame struct{
 Revision uint64 `json:"revision"`
 Before string `json:"before"`
 After string `json:"after"`
 Submissions []game.Submission `json:"submissions"`
 Retrieval []RetrievalReceipt `json:"retrieval"`
}
type Run struct{
 Version string `json:"version"`
 Model string `json:"model"`
 Frames []Frame `json:"frames"`
 Final game.State `json:"final"`
}
var identity = map[string]string{
 "actor.yuuichi-katagiri":"Part-time worker with modest finances; friendships and complex formative experience with money matter. Compare incentives, consider uncertainty and accept local loss only for a justified objective.",
 "actor.tenji-mikasa":"Academic high achiever and mediator. You know your own undisclosed investigative motive concerning your father's death; your suspicion concerning Shiho is not proof. Protect private objectives.",
 "actor.shiho-sawaragi":"Class vice-president with a strong fairness orientation. Prefer honest cooperation; challenge unsupported allegations; trust is revisable.",
 "actor.makoto-shibe":"Class president with wealthy family background. Family loyalty and social acceptance matter; you may make impulsive commitments.",
 "actor.yutori-kokorogi":"Shy public presentation and apparent dependence on friends. Keep your own undisclosed intentions separate from what others observe; do not assume others know them.",
}
func Digest(s game.State)(string,error){
 _,h,err:=game.CandidateBytes(s);if err!=nil{return "",err};return hex.EncodeToString(h[:]),nil
}
func validateGuidance(actor string,g Guidance)error{
 if len(g.Guidance)!=len(g.EvidenceIDs)||len(g.Guidance)>5||len(g.ManifestDigest)!=64{return errors.New("invalid retrieval receipt")}
 if _,err:=hex.DecodeString(g.ManifestDigest);err!=nil{return errors.New("invalid retrieval digest")}
 seen:=map[string]bool{}
 prefix:="ev.tg.guidance."+strings.TrimPrefix(actor,"actor.")+"."
 for i,id:=range g.EvidenceIDs{
  if !strings.HasPrefix(id,prefix)||seen[id]||len(g.Guidance[i])==0||len(g.Guidance[i])>1000{return errors.New("foreign or invalid guidance")}
  seen[id]=true
 }
 return nil
}
func query(v game.View)string{
 // Never query with owner state, other actors' ballots or private cards.
 parts:=[]string{v.Game,v.Phase,v.Question,"strategy trust cooperation reasoning"}
 start:=len(v.Messages)-3;if start<0{start=0}
 for _,m:=range v.Messages[start:]{parts=append(parts,m.Text)}
 return strings.Join(parts," ")
}
func Execute(ctx context.Context,modelName string,retrieve Retrieve,decide Model)(Run,error){
 if retrieve==nil||decide==nil||modelName==""{return Run{},errors.New("model and retrieval required")}
 s:=game.New();run:=Run{Version:game.Version,Model:modelName,Frames:[]Frame{}}
 corpusDigest:=""
 for s.Phase!=game.Terminal{
  if err:=ctx.Err();err!=nil{return run,err}
  before,err:=Digest(s);if err!=nil{return run,err}
  frame:=Frame{Revision:s.Revision,Before:before,Submissions:[]game.Submission{},Retrieval:[]RetrievalReceipt{}}
  // All decisions use the same frozen revision. Previous decisions from this
  // cohort are never available to the next model invocation.
  for _,actor:=range s.Actors{
   v,err:=game.Project(s,actor.ID);if err!=nil{return run,err}
   g,err:=retrieve(ctx,actor.ID,v.Game,query(v));if err!=nil{return run,fmt.Errorf("retrieval for %s: %w",actor.ID,err)}
   if err=validateGuidance(actor.ID,g);err!=nil{return run,err}
   if corpusDigest==""{corpusDigest=g.ManifestDigest}else if g.ManifestDigest!=corpusDigest{return run,errors.New("corpus changed during run")}
   d,err:=decide(ctx,Input{v,identity[actor.ID],append([]string{},g.Guidance...)})
   if err!=nil{return run,fmt.Errorf("decision for %s: %w",actor.ID,err)}
   if err=game.ValidateDecision(v,d);err!=nil{return run,fmt.Errorf("decision for %s: %w",actor.ID,err)}
   frame.Submissions=append(frame.Submissions,game.Submission{Actor:actor.ID,Decision:d})
   frame.Retrieval=append(frame.Retrieval,RetrievalReceipt{actor.ID,append([]string{},g.EvidenceIDs...),g.ManifestDigest})
  }
  n,err:=game.Apply(s,s.Revision,frame.Submissions);if err!=nil{return run,err}
  frame.After,err=Digest(n);if err!=nil{return run,err}
  run.Frames=append(run.Frames,frame);s=n
 }
 run.Final=s
 return run,nil
}
func Replay(run Run)error{
 if run.Version!=game.Version||run.Model==""||len(run.Frames)==0||len(run.Frames)>10{return errors.New("unsupported run")}
 s:=game.New();manifest:=""
 for _,f:=range run.Frames{
  d,err:=Digest(s);if err!=nil||d!=f.Before||f.Revision!=s.Revision{return errors.New("predecessor mismatch")}
  if len(f.Retrieval)!=game.Players{return errors.New("missing retrieval receipts")}
  for i,r:=range f.Retrieval{
   if r.Actor!=s.Actors[i].ID||len(r.ManifestDigest)!=64||len(r.EvidenceIDs)>5{return errors.New("invalid retrieval receipt")}
   if _,err:=hex.DecodeString(r.ManifestDigest);err!=nil{return err}
   if manifest==""{manifest=r.ManifestDigest}else if manifest!=r.ManifestDigest{return errors.New("corpus digest mismatch")}
   seen:=map[string]bool{}
   for _,id:=range r.EvidenceIDs{if !strings.HasPrefix(id,"ev.tg.guidance."+strings.TrimPrefix(r.Actor,"actor.")+".")||seen[id]{return errors.New("foreign retrieval receipt")};seen[id]=true}
  }
  n,err:=game.Apply(s,f.Revision,f.Submissions);if err!=nil{return err}
  d,err=Digest(n);if err!=nil||d!=f.After{return errors.New("successor mismatch")};s=n
 }
 if s.Phase!=game.Terminal{return errors.New("incomplete run")}
 actual,err:=Digest(s);if err!=nil{return err};want,err:=Digest(run.Final);if err!=nil||actual!=want{return errors.New("final state mismatch")}
 return nil
}
