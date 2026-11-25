package pilot

import (
 "context"
 "errors"
 "time"
)

// Quiet private recovery may span two ordinary 60-minute rest segments,
// but never across needs, a public boundary, an exam, a live commitment,
// an active plan, an addressed stimulus, a contact or an observed arrival.
func quietRestDuration(j Journal,s State,id string)int64{
 m:=s.Minds[id]
 if m.Location!="dormitory-"+id||m.Body==nil||m.PendingStimulus!=""||m.Conversation!=nil||m.NextDeliberation>0||m.Plan!=nil||len(m.Plans)>0{return 60}
 if e:=s.World.Exam;e!=nil&&e.Outcome==0&&has(e.Group,id){return 60}
 for _,mt:=range s.World.Meetings{if (mt.Status=="pending"||mt.Status=="accepted")&&(mt.Host==id||mt.Guest==id){return 60}}
 for _,peer:=range ids(j.Profiles){if peer==id{continue};p:=s.Minds[peer];if p.Location==m.Location||p.Activity!=nil&&p.Activity.Kind=="move"&&p.Activity.Target==m.Location{return 60}}
 n:=int64(120)
 for _,cap:=range []int64{nextDue(j,s)-s.Minute,int64(m.Body.Energy)-350,int64(m.Body.Hydration)-350}{if cap<n{n=cap}}
 if n<60{return 60}
 return n
}
func requiresDetailedCycle(j Journal,s State)bool{
 for _,id:=range eligibleActors(j,s){if decisionNeedsModel(j,s,id){return true}}
 return false
}
func meaningfulBoundary(events []Event)bool{
 for _,e:=range events{if has([]string{"claim","meeting","exam","revelation","goal","plan","plan_end","belief","relation","social","body_alert","discovery","conversation_end","resolution","resource","obstacle"},e.Kind){return true}}
 return false
}

// Compute a bounded macro of normal canonical rounds. There is no new
// source of decisions or alternate reducer. Stop before inference or after
// the first meaningful boundary. The caller commits the complete journal
// atomically; each round remains individually replayable.
func routineMacro(ctx context.Context,j Journal,s State,target int64,limit int)(State,[]Round,string,error){
 rounds:=[]Round{}
 for len(rounds)<limit{
  if err:=ctx.Err();err!=nil{return s,nil,"",err}
  if s.Minute>=target{return s,rounds,"Data de destino atingida",nil}
  if s.Round>=roundLimit(j.Version)||s.Minute>=endMinute(j.Version){return s,rounds,"Limite do piloto atingido",nil}
  if requiresDetailedCycle(j,s){return s,rounds,"Decisão cognitiva necessária",nil}
  choices:=[]Chosen{};for _,id:=range eligibleActors(j,s){choices=append(choices,Chosen{Actor:id,Decision:routineDecision(j,s,id)})}
  next,err:=scheduledAdvance(j,s,choices);if err!=nil{return s,nil,"",err}
  rounds=append(rounds,Round{Number:next.Round,Minute:next.Minute,Choices:choices})
  relevant:=meaningfulBoundary(next.Events[len(s.Events):]);s=next
  if relevant{return s,rounds,"Acontecimento relevante registrado; reavaliar resolução",nil}
 }
 return s,rounds,"Lote de rotinas concluído",nil
}
func(s *Session) advanceRoutineMacro(ctx context.Context,expected int,target int64)(result AdvanceResult,retErr error){
 if !s.stepMu.TryLock(){return result,errors.New("uma rodada já está em execução")};defer s.stepMu.Unlock()
 s.mu.Lock()
 if s.lock==nil||s.state.Round!=expected||len(s.pending)>0{s.mu.Unlock();return result,errors.New("sessão alterada ou decisão pendente")}
 if target<=s.state.Minute||target>endMinute(s.journal.Version)||target%1440!=480{s.mu.Unlock();return result,errors.New("data de destino inválida")}
 ctx,cancel:=context.WithCancel(ctx);s.cancel=cancel
 j,state:=s.journal,s.state
 s.progress=Progress{Active:true,Round:expected+1,StartedAt:time.Now(),Phase:"routine-batch"}
 s.mu.Unlock()
 defer func(){cancel();s.mu.Lock();defer s.mu.Unlock();s.cancel=nil;s.progress.Active=false;s.progress.Phase="idle";if retErr!=nil{s.progress.LastError=retErr.Error()}}()
 next,rounds,reason,err:=routineMacro(ctx,j,state,target,32);if err!=nil{return result,err}
 s.mu.Lock();defer s.mu.Unlock()
 if err:=ctx.Err();err!=nil{return result,err}
 if len(rounds)>0{
  j.Rounds=append(append([]Round(nil),j.Rounds...),rounds...)
  s.progress.Phase="saving"
  if err:=replaceJournal(s.path,j);err!=nil{return result,errors.New("falha ao salvar lote; nenhum ciclo do lote publicado")}
  s.journal=j;s.state=next
 }
 return AdvanceResult{Snapshot:s.snapshot(),Cycles:len(rounds),Reason:reason},nil
}
