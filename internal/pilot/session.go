package pilot

import (
	"context"
 "crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

//go:embed profiles.json
var defaultProfiles []byte

type Session struct {
	mu sync.Mutex
	stepMu sync.Mutex
	progress Progress
	cancel context.CancelFunc
	pending []Chosen
	path string
	journal Journal
	state State
	model Model
	lock *os.File
}

type Progress struct {
	Active bool `json:"active"`
	Round int `json:"round"`
	Actor string `json:"actor"`
	Attempt int `json:"attempt"`
	Completed int `json:"completed"`
	Total int `json:"total"`
	StartedAt time.Time `json:"started_at"`
	ActorStartedAt time.Time `json:"actor_started_at"`
	Phase string `json:"phase"`
	LastError string `json:"last_error,omitempty"`
	Choices []Chosen `json:"choices"`
}
func (s *Session) Progress() Progress {
	s.mu.Lock();defer s.mu.Unlock()
	p:=s.progress
	p.Choices=clone(p.Choices)
	return p
}
func (s *Session) Cancel(round int) bool {
	s.mu.Lock();defer s.mu.Unlock()
	if !s.progress.Active||s.progress.Round!=round||s.cancel==nil||s.progress.Phase=="saving"{return false}
	s.cancel()
	s.progress.Phase="canceling"
	return true
}

type Snapshot struct {
	State State `json:"state"`
	People []Person `json:"people"`
	Model string `json:"model"`
	Digest string `json:"digest"`
	Limit int `json:"limit"`
	Done bool `json:"done"`
	Version int `json:"version"`
	LastChoices []Chosen `json:"last_choices"`
 Population map[string]PublicMember `json:"population,omitempty"`
 Identities map[string]CharacterIdentity `json:"identities,omitempty"`
 NeedsInference bool `json:"needs_inference,omitempty"`
}

func Open(path,name string,model Model) (*Session,error) {return openVersion(path,name,model,Version)}
// OpenScheduled creates v5 saves; existing saves retain their original semantics.
func OpenScheduled(path,name string,model Model) (*Session,error) {return openVersion(path,name,model,WorldVersion)}
func OpenPopulation(path,name string,model Model)(*Session,error){return openVersion(path,name,model,PopulationVersion)}
func OpenCognition(path,name string,model Model)(*Session,error){return openVersion(path,name,model,CognitionVersion)}
// OpenSelective creates v8 saves; existing journals retain their original policies.
func OpenSelective(path,name string,model Model)(*Session,error){return openVersion(path,name,model,SelectiveVersion)}
// OpenAutonomy creates v9 journals; earlier journals are replayed unchanged.
func OpenAutonomy(path,name string,model Model)(*Session,error){return openVersion(path,name,model,AutonomyVersion)}
func OpenStaffCare(path,name string,model Model)(*Session,error){return openVersion(path,name,model,StaffCareVersion)}
func OpenContact(path,name string,model Model)(*Session,error){return openVersion(path,name,model,ContactVersion)}
func OpenMotivation(path,name string,model Model)(*Session,error){return openVersion(path,name,model,MotivationVersion)}
func OpenObservation(path,name string,model Model)(*Session,error){return openVersion(path,name,model,ObservationVersion)}
func OpenVeteran(path,name string,model Model)(*Session,error){return openVersion(path,name,model,VeteranVersion)}
func OpenPhysical(path,name string,model Model)(*Session,error){return openVersion(path,name,model,PhysicalVersion)}
func OpenCompactContext(path,name string,model Model)(*Session,error){return openVersion(path,name,model,CompactContextVersion)}
func OpenCalibrated(path,name string,model Model)(*Session,error){return openVersion(path,name,model,CalibratedVersion)}
func OpenCrossover(path,name string,model Model)(*Session,error){return openVersion(path,name,model,CrossoverVersion)}
func OpenCalendar(path,name string,model Model)(*Session,error){return openVersion(path,name,model,CalendarVersion)}
func OpenPortfolio(path,name string,model Model)(*Session,error){return openVersion(path,name,model,PortfolioVersion)}
func OpenMultiresolution(path,name string,model Model)(*Session,error){return openVersion(path,name,model,MultiresolutionVersion)}
func openVersion(path,name string,model Model,version int) (*Session,error) {
	if model==nil||!validText(name,200){return nil,errors.New("model required")}
	path,err:=filepath.Abs(path);if err!=nil{return nil,err}
	parent,err:=filepath.EvalSymlinks(filepath.Dir(path));if err!=nil{return nil,err}
	path=filepath.Join(parent,filepath.Base(path))
	lock,err:=os.OpenFile(path+".lock",os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW,0600)
	if err!=nil{return nil,errors.New("cannot open local session lock")}
	if err:=syscall.Flock(int(lock.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{lock.Close();return nil,errors.New("save already open in another process")}
	opened:=false
	defer func(){if !opened{lock.Close()}}()
	j:=Journal{}
	info,err:=os.Lstat(path)
	switch {
	case errors.Is(err,os.ErrNotExist):
		profiles,err:=decodeStrict[[]Profile](defaultProfiles);if err!=nil{return nil,err}
		j=Journal{Version:version,Profiles:profiles,Model:name,Rounds:[]Round{}}
  if version>=PopulationVersion{if err:=expandPopulation(&j);err!=nil{return nil,err}}
  if version>=CrossoverVersion{if err:=admitExternalStudents(&j);err!=nil{return nil,err}}
  if version>=CognitionVersion{initializeIdentities(&j)}
		if version>=WorldVersion{seed,err:=randomSecret();if err!=nil{return nil,err};sum:=sha256.Sum256([]byte(seed));j.WorldSeed=fmt.Sprintf("%x",sum)}
		if _,err:=Replay(j);err!=nil{return nil,err}
		if err:=createJournal(path,j);err!=nil{return nil,err}
	case err!=nil:
		return nil,err
	default:
		if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||info.Size()>32<<20{return nil,errors.New("invalid or exposed save file")}
		f,err:=os.Open(path);if err!=nil{return nil,err}
		raw,err:=io.ReadAll(io.LimitReader(f,32<<20+1));closeErr:=f.Close()
		if err!=nil||closeErr!=nil||len(raw)>32<<20{return nil,errors.New("cannot read save")}
		j,err=decodeStrict[Journal](raw);if err!=nil{return nil,errors.New("invalid save JSON")}
		if _,routed:=model.(*Router);j.Model!=name&&!routed{return nil,errors.New("save uses a different model; select its original model or a new save")}
	}
	s,err:=Replay(j);if err!=nil{return nil,err}
	opened=true
	return &Session{path:path,journal:j,state:s,model:model,lock:lock},nil
}

func (s *Session) Close() error {
	s.stepMu.Lock();defer s.stepMu.Unlock()
	s.mu.Lock();defer s.mu.Unlock()
	if s.lock==nil{return nil}
	err:=s.lock.Close();s.lock=nil
	return err
}

func writeJournal(file *os.File,j Journal) error {
	data,err:=json.MarshalIndent(j,"","  ");if err!=nil{return err}
	if len(data)>32<<20{return errors.New("save size limit reached")}
	if _,err=file.Write(append(data,'\n'));err!=nil{return err}
	return file.Sync()
}
func createJournal(path string,j Journal) error {
	// Exclusive initial publication, same filesystem as the durable save.
	tmp,err:=os.CreateTemp(filepath.Dir(path),".cote-new-");if err!=nil{return err}
	defer os.Remove(tmp.Name())
	err=writeJournal(tmp,j);closeErr:=tmp.Close()
	if err!=nil{return err};if closeErr!=nil{return closeErr}
	return os.Link(tmp.Name(),path)
}
func replaceJournal(path string,j Journal) error {
	tmp,err:=os.CreateTemp(filepath.Dir(path),".cote-save-");if err!=nil{return err}
	defer os.Remove(tmp.Name())
	err=writeJournal(tmp,j);closeErr:=tmp.Close()
	if err!=nil{return err};if closeErr!=nil{return closeErr}
	// Readers see either the previous complete round or the new complete round.
	return os.Rename(tmp.Name(),path)
}
func (s *Session) snapshot() Snapshot {
	people:=[]Person{}
	for _, p:=range s.journal.Profiles {people=append(people,Person{p.ID,p.Name,p.Class})}
	last:=[]Chosen{}
	if n:=len(s.journal.Rounds);n>0{last=clone(s.journal.Rounds[n-1].Choices)}
	return Snapshot{Identities:clone(s.journal.Identities),NeedsInference:s.needsInferenceLocked(),Population:publicPopulation(s.journal,s.state),LastChoices:last,State:clone(s.state),People:people,Model:s.journal.Model,Digest:Digest(s.journal),Limit:roundLimit(s.journal.Version),Done:s.state.Round>=roundLimit(s.journal.Version)||(s.journal.Version>=Version&&s.state.Minute>=endMinute(s.journal.Version)),Version:s.journal.Version}
}
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock();defer s.mu.Unlock();return s.snapshot()
}
func (s *Session) POV(id string) (Context,error) {
	s.mu.Lock();defer s.mu.Unlock()
	if !known(s.journal.Profiles,id){return Context{},errors.New("unknown actor")}
	return actorContext(s.journal,s.state,id),nil
}
func(s *Session) needsInferenceLocked()bool{
 for _,id:=range eligibleActors(s.journal,s.state){if s.journal.Version<PopulationVersion||decisionNeedsModel(s.journal,s.state,id){return true}}
 return false
}
func (s *Session) Step(ctx context.Context,expected int)(Snapshot,error){return s.step(ctx,expected,true)}
func (s *Session) step(ctx context.Context,expected int,allowModel bool) (result Snapshot,retErr error) {
	if !s.stepMu.TryLock(){return Snapshot{},errors.New("uma rodada já está em execução")}
	defer s.stepMu.Unlock()
	s.mu.Lock()
	if s.lock==nil{s.mu.Unlock();return Snapshot{},errors.New("sessão encerrada")}
	if s.state.Round!=expected{s.mu.Unlock();return Snapshot{},errors.New("rodada já alterada; atualize o estado")}
 if !allowModel&&(s.needsInferenceLocked()||len(s.pending)>0){s.mu.Unlock();return Snapshot{},errors.New("Há decisão cognitiva pendente; use Executar ou Próximo ciclo.")}
	if expected>=roundLimit(s.journal.Version)||(s.journal.Version>=Version&&s.state.Minute>=endMinute(s.journal.Version)){s.mu.Unlock();return Snapshot{},errors.New("período da simulação concluído")}
	ctx,cancel:=context.WithCancel(ctx)
	s.cancel=cancel
	order:=eligibleActors(s.journal,s.state)
	s.progress=Progress{Active:true,Round:expected+1,Total:len(order),Completed:len(s.pending),StartedAt:time.Now(),Phase:"thinking",Choices:clone(s.pending)}
	journal,state:=s.journal,s.state
	s.mu.Unlock()
	defer func(){
		cancel()
		s.mu.Lock();defer s.mu.Unlock()
		s.cancel=nil;s.progress.Active=false;s.progress.Actor="";s.progress.Phase="idle"
		if retErr!=nil{s.progress.LastError=retErr.Error()}
	}()
	var err error
	r:=Round{Number:expected+1,Choices:append([]Chosen{},s.pending...)}
	for _, id:=range order[len(r.Choices):] {
		if err:=ctx.Err();err!=nil{return Snapshot{},err}
		view:=actorContext(journal,state,id)
		var problem string
		var d Decision
		failure:=""
  if journal.Version>=PopulationVersion&&!decisionNeedsModel(journal,state,id){
   s.mu.Lock();s.progress.Actor=id;s.progress.Attempt=0;s.progress.ActorStartedAt=time.Now();s.progress.LastError="";s.mu.Unlock()
   d=routineDecision(journal,state,id)
   if err:=validateDecision(view,d);err!=nil{return Snapshot{},fmt.Errorf("rotina %s: %w",id,err)}
  }else{
		for attempt:=1;attempt<=3;attempt++ {
			if err:=ctx.Err();err!=nil{return Snapshot{},err}
			s.mu.Lock()
			if ctx.Err()!=nil{s.mu.Unlock();return Snapshot{},ctx.Err()}
			s.progress.Actor=id;s.progress.Attempt=attempt;s.progress.ActorStartedAt=time.Now();s.progress.LastError=problem
			s.mu.Unlock()
			if attempt==1 {d,err=s.model.Decide(ctx,view)} else {
				d,err=s.model.(interface{Repair(context.Context,Context,string)(Decision,error)}).Repair(ctx,view,problem)
			}
			if ctx.Err()!=nil{return Snapshot{},ctx.Err()}
			retryable:=false
			if err==nil {err=validateDecision(view,d);if journal.Version>=PopulationVersion&&d.Source==routineSource{err=errors.New("model cannot impersonate deterministic routine")};retryable=err!=nil} else {
				var malformed responseError
				retryable=errors.As(err,&malformed)
			}
			if err==nil {break}
			problem=err.Error()
			_,repairable:=s.model.(interface{Repair(context.Context,Context,string)(Decision,error)})
			if !repairable||!retryable{return Snapshot{},fmt.Errorf("%s: %w",id,err)}
			if attempt==3 {
				if journal.Version>=ScheduledVersion{return Snapshot{},fmt.Errorf("%s: %w",id,err)}
				failure="Modelo não produziu uma ação válida após três tentativas: "+problem
				d=Decision{Action:"wait",Reason:"Sem decisão válida do modelo",Beliefs:[]Belief{}}
			}
		}
		}
  chosen:=Chosen{Actor:id,Decision:clone(d),Failure:failure}
		r.Choices=append(r.Choices,chosen)
		s.pending=append(s.pending,chosen)
		s.mu.Lock();s.progress.Completed=len(r.Choices);s.progress.Choices=clone(r.Choices);s.progress.LastError="";s.mu.Unlock()
	}
	valid:=0
	for _,x:=range r.Choices{if x.Failure==""{valid++}}
	if valid==0&&len(order)>0 {
		s.mu.Lock();s.pending=nil;s.mu.Unlock()
		return Snapshot{},errors.New("nenhum agente produziu decisão válida; rodada não avançou")
	}
	r.Minute,err=nextEventMinute(journal,state,r.Choices);if err!=nil{return Snapshot{},err}
	next,err:=apply(journal,state,r);if err!=nil{return Snapshot{},err}
	j:=journal
	j.Rounds=append(append([]Round(nil),j.Rounds...),r)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err:=ctx.Err();err!=nil{return Snapshot{},err}
	s.progress.Phase="saving"
	if err:=replaceJournal(s.path,j);err!=nil{return Snapshot{},errors.New("falha ao salvar rodada; estado não publicado")}
	s.journal=j;s.state=next;s.pending=nil
	return s.snapshot(),nil
}

type AdvanceResult struct{
 Snapshot Snapshot `json:"snapshot"`
 Reason string `json:"reason"`
 Cycles int `json:"cycles"`
}
// AdvanceUntilEvent commits exactly the normal deterministic cycles and
// stops before any inference. Chunking bounds request duration, not sim time.
func(s *Session) AdvanceUntilEvent(ctx context.Context,expected int)(AdvanceResult,error){
 result:=AdvanceResult{Snapshot:s.Snapshot()}
 if result.Snapshot.Version<CognitionVersion{return result,errors.New("avanço por acontecimento requer v7")}
 if result.Snapshot.State.Round!=expected{return result,errors.New("rodada já alterada")}
 for i:=0;i<64;i++{
  if result.Snapshot.Done{result.Reason="Limite do piloto atingido";return result,nil}
  if result.Snapshot.NeedsInference{result.Reason="Decisão cognitiva necessária";return result,nil}
  prev:=len(result.Snapshot.State.Events)
  next,err:=s.step(ctx,result.Snapshot.State.Round,false);if err!=nil{return result,err}
  result.Snapshot=next;result.Cycles++
  for _,e:=range next.State.Events[prev:]{
   if has([]string{"revelation","claim","meeting","exam","plan_end","body_alert","social"},e.Kind){result.Reason="Novo acontecimento: "+e.Kind;return result,nil}
  }
 }
 result.Reason="Rotinas avançadas; nenhum acontecimento novo neste lote"
 return result,nil
}

 // AdvanceRoutineTo advances only the normal, committed scheduler cycles.
 // It stops before an inference request and never creates substitute dialogue.
 func(s *Session) AdvanceRoutineTo(ctx context.Context,expected int,target int64)(AdvanceResult,error){
  result:=AdvanceResult{Snapshot:s.Snapshot()}
  if result.Snapshot.Version<CognitionVersion{return result,errors.New("avanço por data requer v7")}
  if result.Snapshot.State.Round!=expected{return result,errors.New("rodada já alterada")}
  if result.Snapshot.Version>=MultiresolutionVersion{return s.advanceRoutineMacro(ctx,expected,target)}
  if target<=result.Snapshot.State.Minute||target>result.Snapshot.State.Minute+7*1440+480||target>endMinute(result.Snapshot.Version)||target%1440!=480{return result,errors.New("data de destino inválida")}
  for i:=0;i<64;i++{
   if err:=ctx.Err();err!=nil{return result,err}
   if result.Snapshot.State.Minute>=target{result.Reason="Data de destino atingida";return result,nil}
   if result.Snapshot.Done{result.Reason="Limite do piloto atingido";return result,nil}
   if result.Snapshot.NeedsInference{result.Reason="Decisão cognitiva necessária; nenhuma decisão substituída";return result,nil}
   next,err:=s.step(ctx,result.Snapshot.State.Round,false);if err!=nil{return result,err}
   result.Snapshot=next;result.Cycles++
  }
  if result.Snapshot.State.Minute>=target{result.Reason="Data de destino atingida"}else{result.Reason="Lote de rotinas concluído"}
  return result,nil
 }
