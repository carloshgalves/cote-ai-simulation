// Package tomodachi contains a pure, proposed CSF exam-domain reducer.
// It has no persistence or LLM authority. Native CommitCoordinator registration
// is deliberately not implied by the existence of this package.
package tomodachi

import (
 "errors"
 "fmt"
 "strings"
 "unicode/utf8"
)

const Version = "tg.kokkuri.model.v1"
const InitialDebt int64 = 4000000
const Prize int64 = 2000000
const MaxDebt int64 = 1<<50
const Players = 5
const Discussion = "discussion"
const Ballot = "ballot"
const Terminal = "terminal"

type Actor struct { ID string `json:"id"`; Debt int64 `json:"debt"`; Votes []string `json:"votes"` }
type Message struct { Round int `json:"round"`; From string `json:"from"`; Text string `json:"text"` }
type State struct {
 Version string `json:"version"`
 Revision uint64 `json:"revision"`
 Round int `json:"round"`
 Phase string `json:"phase"`
 Actors [Players]Actor `json:"actors"`
 Messages []Message `json:"messages"`
 Results []string `json:"results"`
 Winners []string `json:"winners"`
}
type Decision struct { Choice string `json:"choice"`; Text string `json:"text,omitempty"` }
type Submission struct { Actor string `json:"actor"`; Decision Decision `json:"decision"` }
type View struct {
 Game string `json:"game"`
 Actor string `json:"actor"`
 Round int `json:"round"`
 Phase string `json:"phase"`
 Questioner string `json:"questioner"`
 Question string `json:"question"`
 OwnDebt int64 `json:"own_debt"`
 OwnVotes []string `json:"own_votes"`
 Roster []string `json:"roster"`
 Messages []Message `json:"messages"`
 Results []string `json:"results"`
 PrivateCard string `json:"private_card,omitempty"`
 Rules []string `json:"rules"`
 Choices []string `json:"choices"`
}
var roster = [Players]string{"actor.makoto-shibe","actor.shiho-sawaragi","actor.tenji-mikasa","actor.yuuichi-katagiri","actor.yutori-kokorogi"}
var questions = [Players]string{
 "Are there five contestants in this group?",
 "Is the debt denominated in yen?",
 "Is this a game with simultaneous yes/no choices?",
 "Can an incorrect result advance us to another question?",
 "Do you intend to remain friends?",
}
func New() State {
 s:=State{Version:Version,Round:1,Phase:Discussion,Messages:[]Message{},Results:[]string{},Winners:[]string{}}
 for i,id:=range roster {s.Actors[i]=Actor{ID:id,Debt:InitialDebt,Votes:[]string{}}}
 return s
}
func copyState(s State) State {
 s.Messages=append([]Message{},s.Messages...)
 s.Results=append([]string{},s.Results...)
 s.Winners=append([]string{},s.Winners...)
 for i:=range s.Actors {s.Actors[i].Votes=append([]string{},s.Actors[i].Votes...)}
 return s
}
func (s State) Validate() error {
 if s.Version!=Version || s.Round<1 || s.Round>Players || (s.Phase!=Discussion && s.Phase!=Ballot && s.Phase!=Terminal) {return errors.New("invalid exam state")}
 completed:=s.Round-1
 if s.Phase==Terminal {completed=s.Round}
 expected:=uint64(2*completed)
 if s.Phase==Ballot {expected++}
 if s.Revision!=expected || len(s.Results)!=completed || len(s.Messages)>Players*Players {return errors.New("invalid revision/history")}
 if s.Phase!=Terminal && len(s.Winners)!=0 {return errors.New("nonterminal winners")}
 for i,a:=range s.Actors{
  if a.ID!=roster[i] || a.Debt<0 || a.Debt>MaxDebt || len(a.Votes)!=completed {return errors.New("invalid actor state")}
  for _,v:=range a.Votes {if v!="yes"&&v!="no"{return errors.New("invalid recorded ballot")}}
 }
 for _,v:=range s.Results {if v!="yes"&&v!="no"{return errors.New("invalid recorded result")}}
 for _,m:=range s.Messages {if m.Round<1||m.Round>s.Round||!known(m.From)||!validText(m.Text){return errors.New("invalid message history")}}
 if s.Phase==Terminal{
  if s.Results[len(s.Results)-1]=="yes" {if len(s.Winners)!=Players{return errors.New("missing winners")};for i,id:=range roster{if s.Winners[i]!=id{return errors.New("invalid winner")}}} else if s.Round!=Players||len(s.Winners)!=0 {return errors.New("invalid failed terminal")}
 }
 return nil
}
func known(id string)bool{for _,a:=range roster{if a==id{return true}};return false}
func validText(s string)bool{return utf8.ValidString(s)&&len(s)<=800&&strings.TrimSpace(s)!=""}
func Project(s State,id string)(View,error){
 if err:=s.Validate();err!=nil{return View{},err}
 if !known(id)||s.Phase==Terminal{return View{},errors.New("invalid active actor")}
 v:=View{Game:"kokkuri-san",Actor:id,Round:s.Round,Phase:s.Phase,Questioner:roster[s.Round-1],Question:questions[s.Round-1],
  Roster:append([]string{},roster[:]...),Messages:append([]Message{},s.Messages...),Results:append([]string{},s.Results...),
  Rules:[]string{"All votes are simultaneous and individual votes stay private.","If votes differ, the minority answer is selected. Otherwise the unanimous answer is selected.","A correct answer clears the game and reduces group debt by two million yen, split equally.","Speaking during the discussion phase when not the questioner doubles your debt. A message is a claim, not a verified fact.","After five incorrect results, the largest debtors absorb total remaining debt. Ties split the debt."}}
 for _,a:=range s.Actors{if a.ID==id{v.OwnDebt=a.Debt;v.OwnVotes=append([]string{},a.Votes...)}}
 if v.Questioner==id&&s.Round<Players{v.PrivateCard="An incorrect no result halves your own debt. Other players have not been shown this card."}
 if s.Phase==Discussion{v.Choices=[]string{"pass","talk"}}else{v.Choices=[]string{"yes","no"}}
 return v,nil
}
func ValidateDecision(v View,d Decision)error{
 if v.Phase==Discussion{
  if d.Choice=="pass"&&d.Text==""{return nil}
  if d.Choice=="talk"&&validText(d.Text){return nil}
 } else if v.Phase==Ballot&&(d.Choice=="yes"||d.Choice=="no")&&d.Text=="" {return nil}
 return errors.New("illegal exam decision")
}
// Apply is deterministic over one frozen state and a complete cohort. It
// returns a proposed successor, never commits it. Submission order is irrelevant.
func Apply(before State, expected uint64, submissions []Submission)(State,error){
 if err:=before.Validate();err!=nil{return State{},err}
 if before.Phase==Terminal||expected!=before.Revision||len(submissions)!=Players{return State{},errors.New("stale, terminal or incomplete cohort")}
 byID:=map[string]Decision{}
 for _,sub:=range submissions{
  if _,exists:=byID[sub.Actor];exists{return State{},errors.New("duplicate actor")}
  v,err:=Project(before,sub.Actor);if err!=nil{return State{},err}
  if err=ValidateDecision(v,sub.Decision);err!=nil{return State{},err}
  byID[sub.Actor]=sub.Decision
 }
 next:=copyState(before)
 next.Revision++
 q:=next.Round-1
 if next.Phase==Discussion{
  for i,id:=range roster{d:=byID[id];if d.Choice=="talk"{
   if i!=q {if next.Actors[i].Debt>MaxDebt/2{return State{},errors.New("debt overflow")};next.Actors[i].Debt*=2}
   next.Messages=append(next.Messages,Message{next.Round,id,d.Text})
  }}
  next.Phase=Ballot
 }else{
  yes:=0
  for i,id:=range roster{answer:=byID[id].Choice;next.Actors[i].Votes=append(next.Actors[i].Votes,answer);if answer=="yes"{yes++}}
  answer:="no";if yes==Players||(yes>0&&yes<Players-yes){answer="yes"}
  next.Results=append(next.Results,answer)
  if answer=="yes"{
   for i:=range next.Actors{d:=next.Actors[i].Debt-Prize/Players;if d<0{d=0};next.Actors[i].Debt=d}
   next.Phase=Terminal;next.Winners=append([]string{},roster[:]...)
  }else{
   if next.Round<Players{next.Actors[q].Debt/=2;next.Round++;next.Phase=Discussion}else{
    var total,max int64
    for _,a:=range next.Actors{total+=a.Debt;if a.Debt>max{max=a.Debt}}
    count:=0;for _,a:=range next.Actors{if a.Debt==max{count++}}
    if total>MaxDebt{return State{},errors.New("aggregate debt overflow")}
    remainder:=total%int64(count)
    for i,a:=range next.Actors{next.Actors[i].Debt=0;if a.Debt==max{next.Actors[i].Debt=total/int64(count);if remainder>0{next.Actors[i].Debt++;remainder--}}}
    next.Phase=Terminal
   }
  }
 }
 if err:=next.Validate();err!=nil{return State{},fmt.Errorf("invalid proposed successor: %w",err)}
 return next,nil
}
