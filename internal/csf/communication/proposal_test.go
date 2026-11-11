package communication

import (
 "bytes"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func signedCommunicationProposal(t *testing.T)(ports.CanonicalRecord,SendSpec,*policy.Bundle){
 t.Helper()
 bundle,claim,tx:=fixture(t)
 var run,cycle,round,dispatchID domain.ID
 run[0]=0x78;cycle[0]=0x79;round[0]=0x80;dispatchID[0]=0x81
 d:=inputledger.Dispatch{ID:dispatchID,RunID:run,CycleID:cycle,RoundID:round,
  SlotID:"act",Actor:[2]string{"school","a"},BaseRevision:0,EffectiveAt:4}
 d.Eligibility.NotBeforeInstant=4
 proposal,err:=testresourcev1.ProposalForDispatch(bundle,d);if err!=nil{t.Fatal(err)}
 s:=SendSpec{Run:run,ActualSender:Actor{"school","a"},PresentedSender:"C",
  Recipient:Actor{"school","b"},Channel:"school.message.v1",SentAt:4,DueAt:12,
  Proposition:claim.Proposition}
 typed,err:=SendParameters(s);if err!=nil{t.Fatal(err)}
 fields:=append([]any(nil),proposal.Body...)
 fields[4]="communication.send"
 fields[5]=uint64(1)
 fields[7]=[]any{}
 fields[8]=typed
 fields[9]=[]any{}
 fields[10]=[]any{uint64(0)}
 dig,_,err:=identity.New(bundle).Sum("cote.csf.digest.action-proposal-unit",
  "cote.csf.schema.action-proposal.body",1,fields)
 if err!=nil{t.Fatal(err)}
 raw,err:=codec.CanonicalPayload(append(fields,dig[:]));if err!=nil{t.Fatal(err)}
 proposal.Digest=dig
 proposal.Canonical=raw
 s.Source=Origin{Kind:5,ID:proposal.ID,Digest:dig}
 _=tx
 return ports.CanonicalRecord{Identity:[32]byte(proposal.ID),Canonical:raw},s,bundle
}

func TestCSFRV17ActionProposalProducesExactAuthoredSend(t *testing.T){
 p,s,bundle:=signedCommunicationProposal(t)
 auth,err:=AuthoredAction(bundle,p)
 if err!=nil{t.Fatal(err)}
 if auth.Spec.ActualSender!=s.ActualSender||auth.Spec.Recipient!=s.Recipient||
  auth.Spec.PresentedSender!="C"||auth.Spec.DueAt!=12||
  auth.Source.ID!=s.Source.ID||auth.Source.Digest!=s.Source.Digest{
  t.Fatal("stored ActionProposal not authoritative for author/source/recipient")
 }
 plan,err:=PrepareSend(bundle,auth.Spec)
 if err!=nil{t.Fatal(err)}
 if _,err:=ValidateOccurrence(bundle,plan);err!=nil{t.Fatal(err)}
 duplicate,err:=AuthoredAction(bundle,p)
 if err!=nil||duplicate.Source!=auth.Source{t.Fatal("proposal replay changed authored message source",err)}
 raw,err:=codec.StrictDecodePayload(p.Canonical);if err!=nil{t.Fatal(err)}
 fields:=append([]any(nil),raw.([]any)...)
 fields[6]=[]any{"school","c"}
 mutated,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 if _,err:=AuthoredAction(bundle,ports.CanonicalRecord{Identity:p.Identity,Canonical:mutated});err==nil{
  t.Fatal("forged author survived canonical digest")
 }
 fields=append([]any(nil),raw.([]any)...)
 fields[8]=[]any{"unregistered",uint64(1),[]byte{0x80},make([]byte,32)}
 tampered,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 if _,err:=AuthoredAction(bundle,ports.CanonicalRecord{Identity:p.Identity,Canonical:tampered});err==nil{
  t.Fatal("unregistered typed params accepted")
 }
 if bytes.Equal(p.Canonical,mutated){t.Fatal("bad fixture")}
}
