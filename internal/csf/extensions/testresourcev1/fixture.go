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
  Snapshot:decision.Snapshot{BaseRevision:7,ResourceKind:"inventory",ResourceID:"last.resource",Available:1,Known:true,
   ForbiddenActors:map[string]bool{"school:a":true}},
  Admitted:[]domain.ID{a.ID,b.ID},Units:[]decision.Unit{{Response:a,ProducerScope:"exam"},{Response:b,ProducerScope:"exam"}}},nil
}
