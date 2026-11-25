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
)

//go:embed profiles.json
var defaultProfiles []byte

type Session struct {
	mu sync.Mutex
	path string
	journal Journal
	state State
	model Model
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
		if !info.Mode().IsRegular()||info.Size()>32<<20{return nil,errors.New("invalid save file")}
		f,err:=os.Open(path);if err!=nil{return nil,err}
		raw,err:=io.ReadAll(io.LimitReader(f,32<<20+1));closeErr:=f.Close()
		if err!=nil||closeErr!=nil||len(raw)>32<<20{return nil,errors.New("cannot read save")}
		j,err=decodeStrict[Journal](raw);if err!=nil{return nil,errors.New("invalid save JSON")}
		if j.Model!=name{return nil,errors.New("save uses a different model; select its original model or a new save")}
	}
	s,err:=Replay(j);if err!=nil{return nil,err}
	return &Session{path:path,journal:j,state:s,model:model},nil
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
func (s *Session) Step(ctx context.Context,expected int) (Snapshot,error) {
	if !s.mu.TryLock(){return Snapshot{},errors.New("uma rodada já está em execução")}
	defer s.mu.Unlock()
	if s.state.Round!=expected{return Snapshot{},errors.New("rodada já alterada; atualize o estado")}
	if expected>=MaxRounds{return Snapshot{},errors.New("limite do piloto atingido")}
	minute,err:=nextMinute(s.state);if err!=nil{return Snapshot{},err}
	r:=Round{Number:expected+1,Minute:minute,Choices:[]Chosen{}}
	for _, id:=range ids(s.journal.Profiles) {
		if err:=ctx.Err();err!=nil{return Snapshot{},err}
		view:=actorContext(s.journal,s.state,id)
		d,err:=s.model.Decide(ctx,view)
		if err!=nil{return Snapshot{},fmt.Errorf("%s: %w",id,err)}
		if err:=validateDecision(view,d);err!=nil{return Snapshot{},fmt.Errorf("%s: %w",id,err)}
		r.Choices=append(r.Choices,Chosen{Actor:id,Decision:d})
	}
	next,err:=apply(s.journal,s.state,r);if err!=nil{return Snapshot{},err}
	if err:=ctx.Err();err!=nil{return Snapshot{},err}
	j:=s.journal
	j.Rounds=append(append([]Round(nil),j.Rounds...),r)
	if err:=replaceJournal(s.path,j);err!=nil{return Snapshot{},errors.New("falha ao salvar rodada; estado não publicado")}
	s.journal=j;s.state=next
	return s.snapshot(),nil
}
