package tomodachi

import (
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// CandidateBytes uses the installed CSF canonical codec for a prospective exam
// value. This domain tag is not yet registered as a WorldState/commit operation.
func CandidateBytes(s State)([]byte,[32]byte,error){
 if err:=s.Validate();err!=nil{return nil,[32]byte{},err}
 texts:=func(v []string)[]any{out:=make([]any,len(v));for i,x:=range v{out[i]=x};return out}
 actors:=make([]any,0,Players)
 for _,a:=range s.Actors{actors=append(actors,[]any{a.ID,a.Debt,texts(a.Votes)})}
 messages:=make([]any,0,len(s.Messages))
 for _,m:=range s.Messages{messages=append(messages,[]any{uint64(m.Round),m.From,m.Text})}
 body:=[]any{s.Version,s.Revision,uint64(s.Round),s.Phase,actors,messages,texts(s.Results),texts(s.Winners)}
 raw,err:=codec.CanonicalBytes("cote.tg.kokkuri.candidate","cote.tg.schema.kokkuri-state",1,body)
 if err!=nil{return nil,[32]byte{},err}
 return raw,identity.OpaqueContentHash(raw),nil
}
