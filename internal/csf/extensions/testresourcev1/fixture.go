// Package testresourcev1 owns the synthetic action schema and scenario data.
// It is a test-only extension, not a game-world economy or a world reducer.
package testresourcev1

import (
 "fmt"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const ActionType="resource.consume"
const SchemaVersion=uint64(1)

func Proposal(bundle *policy.Bundle,run,cycle domain.ID,actor,slot string)(inputledger.Response,error){
 var empty inputledger.Response
 if bundle==nil||domain.ValidateMachineID(actor)!=nil||domain.ValidateMachineID(slot)!=nil{
  return empty,fmt.Errorf("invalid synthetic resource actor/slot or missing pinned bundle")
 }
 var dispatch,round domain.ID
 dispatch[0]=actor[0]
 round[0]=actor[0]+1
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.action-proposal","cote.csf.schema.slot-response-id-preimage",1,[]any{dispatch[:],uint64(0)})
 if err!=nil{return empty,err}
 encoded,err:=codec.CanonicalBytes("cote.csf.testresource.params","cote.csf.schema.typed-value",1,[]any{})
 if err!=nil{return empty,err}
 paramHash:=identity.OpaqueContentHash(encoded)
 fields:=[]any{id[:],dispatch[:],run[:],cycle[:],ActionType,SchemaVersion,
  []any{"school",actor},[]any{},[]any{"cote.csf.schema.typed-value",uint64(1),encoded,paramHash[:]},
  []any{[]any{"inventory","last.resource",uint64(1),int64(1)}},[]any{uint64(0)},int64(100),
  []any{uint64(0)},round[:],slot,uint64(7),[]any{uint64(0)},[]any{uint64(0)}}
 dig,_,err:=d.Sum("cote.csf.digest.action-proposal-unit","cote.csf.schema.action-proposal.body",1,[]any{fields})
 if err!=nil{return empty,err}
 full:=append(append([]any(nil),fields...),dig[:])
 raw,err:=codec.CanonicalPayload(full);if err!=nil{return empty,err}
 return inputledger.Response{Kind:0,ID:domain.ID(id),DispatchID:dispatch,RunID:run,CycleID:cycle,
  RoundID:round,SlotID:slot,Actor:[2]string{"school",actor},EffectiveAt:100,SubmittedAgainstRevision:7,
  Body:fields,Digest:dig,Canonical:raw},nil
}

func LastUnit(bundle *policy.Bundle)(decision.Input,error){
 var run,cycle domain.ID
 var seed domain.Digest
 run[0]=17;cycle[0]=18;seed[0]=19
 a,err:=Proposal(bundle,run,cycle,"a","one");if err!=nil{return decision.Input{},err}
 b,err:=Proposal(bundle,run,cycle,"b","two");if err!=nil{return decision.Input{},err}
 return decision.Input{RunID:run,CycleID:cycle,Attempt:0,Bundle:bundle,Seed:seed,
  Schemas:[]decision.ActionSchema{{Type:ActionType,Version:SchemaVersion,Operation:"resource.consume",Producer:"resource.consume"}},
  Snapshot:decision.Snapshot{BaseRevision:7,ResourceKind:"inventory",ResourceID:"last.resource",Available:1,Known:true,
   AccessibleActors:map[string]bool{"school:a":true,"school:b":true},
   ForbiddenActors:map[string]bool{"school:a":true,"school:b":false}},
  Admitted:[]domain.ID{a.ID,b.ID},Units:[]decision.Unit{{Response:a,ProducerScope:"exam"},{Response:b,ProducerScope:"exam"}}},nil
}

// ProposalForDispatch uses the exact durable dispatch identity, actor, round
// and frozen revision. No handler may invent a slot or rebalance proposals
// based on their arrival order.
func ProposalForDispatch(bundle *policy.Bundle,dispatch inputledger.Dispatch)(inputledger.Response,error){
 r,err:=Proposal(bundle,dispatch.RunID,dispatch.CycleID,dispatch.Actor[1],dispatch.SlotID)
 if err!=nil{return r,err}
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.action-proposal","cote.csf.schema.slot-response-id-preimage",1,[]any{dispatch.ID[:],uint64(0)})
 if err!=nil{return r,err}
 r.Body[0]=id[:]
 r.Body[1]=dispatch.ID[:]
 r.Body[6]=[]any{dispatch.Actor[0],dispatch.Actor[1]}
 r.Body[11]=int64(dispatch.EffectiveAt)
 r.Body[13]=dispatch.RoundID[:]
 r.Body[14]=dispatch.SlotID
 r.Body[15]=uint64(dispatch.BaseRevision)
 dig,_,err:=d.Sum("cote.csf.digest.action-proposal-unit","cote.csf.schema.action-proposal.body",1,[]any{r.Body})
 if err!=nil{return r,err}
 raw,err:=codec.CanonicalPayload(append(append([]any(nil),r.Body...),dig[:]))
 if err!=nil{return r,err}
 r.ID=domain.ID(id);r.DispatchID=dispatch.ID;r.RoundID=dispatch.RoundID
 r.Actor=dispatch.Actor;r.EffectiveAt=dispatch.EffectiveAt
 r.SubmittedAgainstRevision=dispatch.BaseRevision
 r.Canonical=raw;r.Digest=dig
 return r,nil
}
