package pilot

import (
	"context"
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
}
func (s *Session) Progress() Progress {
	s.mu.Lock();defer s.mu.Unlock()
	return s.progress
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
}

func Open(path,name string,model Model) (*Session,error) {
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
		j=Journal{Version:Version,Profiles:profiles,Model:name,Rounds:[]Round{}}
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
		if j.Model!=name{return nil,errors.New("save uses a different model; select its original model or a new save")}
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
	return Snapshot{State:clone(s.state),People:people,Model:s.journal.Model,Digest:Digest(s.journal),Limit:MaxRounds}
}
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock();defer s.mu.Unlock();return s.snapshot()
}
func (s *Session) POV(id string) (Context,error) {
	s.mu.Lock();defer s.mu.Unlock()
	if !known(s.journal.Profiles,id){return Context{},errors.New("unknown actor")}
	return actorContext(s.journal,s.state,id),nil
}
func (s *Session) Step(ctx context.Context,expected int) (result Snapshot,retErr error) {
	if !s.stepMu.TryLock(){return Snapshot{},errors.New("uma rodada já está em execução")}
	defer s.stepMu.Unlock()
	s.mu.Lock()
	if s.lock==nil{s.mu.Unlock();return Snapshot{},errors.New("sessão encerrada")}
	if s.state.Round!=expected{s.mu.Unlock();return Snapshot{},errors.New("rodada já alterada; atualize o estado")}
	if expected>=MaxRounds{s.mu.Unlock();return Snapshot{},errors.New("limite do piloto atingido")}
	ctx,cancel:=context.WithCancel(ctx)
	s.cancel=cancel
	s.progress=Progress{Active:true,Round:expected+1,Total:len(s.journal.Profiles),Completed:len(s.pending),StartedAt:time.Now(),Phase:"thinking"}
	journal,state:=s.journal,s.state
	s.mu.Unlock()
	defer func(){
		cancel()
		s.mu.Lock();defer s.mu.Unlock()
		s.cancel=nil;s.progress.Active=false;s.progress.Actor="";s.progress.Phase="idle"
		if retErr!=nil{s.progress.LastError=retErr.Error()}
	}()
	minute,err:=nextMinute(state);if err!=nil{return Snapshot{},err}
	r:=Round{Number:expected+1,Minute:minute,Choices:append([]Chosen{},s.pending...)}
	order:=ids(journal.Profiles)
	for _, id:=range order[len(r.Choices):] {
		if err:=ctx.Err();err!=nil{return Snapshot{},err}
		view:=actorContext(journal,state,id)
		var problem string
		var d Decision
		for attempt:=1;attempt<=3;attempt++ {
			if err:=ctx.Err();err!=nil{return Snapshot{},err}
			s.mu.Lock()
			s.progress.Actor=id;s.progress.Attempt=attempt;s.progress.ActorStartedAt=time.Now();s.progress.LastError=problem
			s.mu.Unlock()
			if attempt==1 {d,err=s.model.Decide(ctx,view)} else {
				d,err=s.model.(interface{Repair(context.Context,Context,string)(Decision,error)}).Repair(ctx,view,problem)
			}
			if ctx.Err()!=nil{return Snapshot{},ctx.Err()}
			retryable:=false
			if err==nil {err=validateDecision(view,d);retryable=err!=nil} else {
				var malformed responseError
				retryable=errors.As(err,&malformed)
			}
			if err==nil {break}
			problem=err.Error()
			_,repairable:=s.model.(interface{Repair(context.Context,Context,string)(Decision,error)})
			if !repairable||!retryable||attempt==3{return Snapshot{},fmt.Errorf("%s: %w",id,err)}
		}
		r.Choices=append(r.Choices,Chosen{Actor:id,Decision:d})
		s.pending=append(s.pending,Chosen{Actor:id,Decision:d})
		s.mu.Lock();s.progress.Completed=len(r.Choices);s.progress.LastError="";s.mu.Unlock()
	}
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
