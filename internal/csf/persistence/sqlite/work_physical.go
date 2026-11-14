package sqlite

import (
	"bytes"
	"context"
	"errors"
	"strconv"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

type physicalInputProof struct {
	source domain.ID
	digest domain.Digest
	registration policy.PhysicalRegistration
	manifest,cutRaw,signature []byte
	completed []any
}

// verifyPhysicalInput is shared by the live transaction and by durable
// replay, so no public CommitWork request can replace a signed producer
// payload with a caller-authored body state. It checks the Genesis pin
// directly instead of reopening a mutable deployment manifest.
func verifyPhysicalInput(ctx context.Context,q queryOne,run [32]byte,unit []any,
	genesis []any)(physicalInputProof,error){
	var out physicalInputProof
	if len(unit)!=5||unit[1]!=uint64(1){return out,errors.New("physical fence must contain one exogenous source")}
	name,ok:=unit[2].(string)
	if !ok||name!="physical.a1"{return out,errors.New("physical source name unregistered")}
	id,ok:=unit[3].([]byte)
	if !ok||len(id)!=32{return out,errors.New("physical source identity invalid")}
	dig,ok:=unit[4].([]byte)
	if !ok||len(dig)!=32{return out,errors.New("physical source digest invalid")}
	var raw []byte
	if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",id).Scan(&raw);err!=nil{return out,err}
	v,err:=codec.StrictDecodePayload(raw)
	if err!=nil{return out,err}
	fields,ok:=v.([]any)
	if !ok||len(fields)!=13||!equalBytes(fields[0],id)||!equalBytes(fields[1],run[:])||
		fields[2]!=name||fields[4]!="physical.advance"||fields[5]!=uint64(1)||
		!sameCanonical(fields[6],[]any{uint64(0)})||
		!sameCanonical(fields[10],[]any{uint64(0)})||
		!sameCanonical(fields[11],[]any{uint64(0)}){
		return out,errors.New("physical exogenous input identity or schema not registered")
	}
	key,ok:=fields[3].(string)
	if !ok{return out,errors.New("physical producer unit key missing")}
	derivedID,err:=registeredDigest("cote.csf.id.exogenous-input",
		"cote.csf.schema.exogenous-input-id-preimage",[]any{run[:],name,key})
	if err!=nil||!equalBytes(derivedID[:],id){return out,errors.New("physical source ID not derived from ingress")}
	derivedDig,err:=registeredDigest("cote.csf.digest.exogenous-input-unit",
		"cote.csf.schema.exogenous-input.body",fields[:12])
	if err!=nil||!equalBytes(derivedDig[:],dig)||!equalBytes(fields[12],dig){
		return out,errors.New("physical ingress digest not authenticated")
	}
	payload,ok:=fields[7].([]any)
	if !ok||len(payload)!=5||payload[0]!="physical.source-cut.v1"{
		return out,errors.New("physical ingress lacks a closed signed payload")
	}
	manifest,ok:=payload[1].([]byte)
	if !ok{return out,errors.New("physical manifest not canonical bytes")}
	reg,err:=policy.ParsePhysicalManifest(manifest)
	if err!=nil{return out,err}
	if reg.SourceID!=name{return out,errors.New("physical ingress source not in extension")}
	if len(genesis)!=18{return out,errors.New("physical Genesis malformed")}
	extensions,ok:=genesis[12].([]any)
	if !ok||len(extensions)!=1{return out,errors.New("Genesis omitted physical extension pin")}
	extension,ok:=extensions[0].([]any)
	if !ok||len(extension)!=2||extension[0]!=reg.ExtensionID||
		!equalBytes(extension[1],reg.BundleSHA256[:]){
		return out,errors.New("physical manifest not admitted by Genesis")
	}
	sources,ok:=genesis[13].([]any)
	if !ok{return out,errors.New("Genesis physical source set malformed")}
	found:=false
	for _,v:=range sources{if v==reg.SourceID{found=true}}
	if !found{return out,errors.New("physical producer source omitted from Genesis")}
	cutRaw,ok:=payload[2].([]byte)
	if !ok{return out,errors.New("signed source cut bytes missing")}
	signature,ok:=payload[3].([]byte)
	if !ok{return out,errors.New("signed source signature missing")}
	completed,ok:=payload[4].([]any)
	if !ok{return out,errors.New("physical completed state missing")}
	cut,err:=embodimentv1.DecodeSignedCut(cutRaw,signature)
	if err!=nil{return out,err}
	genesisHash,err:=registeredDigest("cote.csf.digest.genesis-manifest",
		"cote.csf.schema.genesis-manifest",genesis)
	if err!=nil{return out,err}
	if err:=embodimentv1.VerifySignedCut(cut,reg,domain.ID(run),genesisHash);err!=nil{return out,err}
	if key!="physical.a1.epoch."+strconv.FormatUint(cut.Epoch,10){
		return out,errors.New("physical ingress producer unit key differs from signed epoch")
	}
	if !equalBytes(completed[3],cut.CompletedStateSHA256[:]){
		return out,errors.New("physical signed body differs from admitted typed state")
	}
	if _,err:=reducer.DecodePhysical(completed);err!=nil{return out,err}
	copy(out.source[:],id)
	copy(out.digest[:],dig)
	out.registration=reg
	out.manifest=bytes.Clone(manifest)
	out.cutRaw=bytes.Clone(cutRaw)
	out.signature=bytes.Clone(signature)
	out.completed=completed
	return out,nil
}

// verifyPhysicalTerminal is the new registered mode 9, called exclusively
// from inside the SAME SQLite transaction as all canonical publications.
// It reconstructs source authority from persisted Genesis and InputLedger,
// recomputes the reducer and every candidate/decision/event/journal linkage.
func verifyPhysicalTerminal(ctx context.Context,q queryOne,b ports.WorkBatch,
	prior,next clock.State,fence,journal,predecessor,revision,genesis []any,
	allowed map[string]any,expectedSlots map[domain.ID]map[string]bool)error{
	if b.Mode!=9||b.Physical==nil||len(expectedSlots)!=0||
		len(b.Events)!=1||len(b.Decisions)!=1||len(b.PerceptionTasks)!=0{
		return errors.New("physical terminal has unsupported partition or perception tasks")
	}
	units,ok:=fence[11].([]any)
	if !ok||len(units)!=1{return errors.New("physical terminal must settle complete single source cut")}
	unit,ok:=units[0].([]any)
	if !ok{return errors.New("physical admitted unit malformed")}
	proof,err:=verifyPhysicalInput(ctx,q,b.RunID,unit,genesis)
	if err!=nil{return err}
	p:=b.Physical
	if !bytes.Equal(p.Manifest,proof.manifest)||!bytes.Equal(p.SignedCut,proof.cutRaw)||
		!bytes.Equal(p.Signature,proof.signature)||
		!sameCanonical(p.CompletedTyped,proof.completed){
		return errors.New("physical WorkBatch differs from durable signed input")
	}
	if p.Bundle!=nil{
		reg,err:=p.Bundle.PhysicalRegistration()
		if err!=nil||reg!=proof.registration{return errors.New("physical builder uses uninstalled policy or divergent Genesis pin")}
	}
	old,ok:=predecessor[4].([]any)
	if !ok{return errors.New("physical predecessor typed state missing")}
	updates,err:=reducer.DecodePhysical(proof.completed)
	if err!=nil{return err}
	after,hash,err:=reducer.ApplyCompletedPhysical(old,updates)
	if err!=nil{return err}
	if !sameCanonical(after,revision[4])||!equalBytes(revision[5],hash[:])||
		!equalBytes(journal[9],hash[:])||b.ResultStateHash!=[32]byte(hash){
		return errors.New("physical world revision is not pure reducer output")
	}
	if prior.Revision==next.Revision||next.Revision!=prior.Revision+1||
		uint64(next.NextLogicalSequence)!=uint64(prior.NextLogicalSequence)+1{
		return errors.New("physical clock and sequence not contiguous")
	}
	if !sameCanonical(journal[11],[]any{proof.source[:]})||
		!sameCanonical(journal[12],fence[12]){
		return errors.New("physical CycleCommit omitted admitted source")
	}
	candidate,err:=strictFull(p.Candidate,17)
	if err!=nil{return err}
	list:=[]any{proof.source[:]}
	expectedID,err:=registeredDigest("cote.csf.id.commit-candidate",
		"cote.csf.schema.candidate-id-preimage",
		[]any{b.RunID[:],b.CycleID[:],list,proof.registration.ActionProducer,
			uint64(proof.registration.ActionVersion),uint64(0),uint64(0)})
	if err!=nil{return err}
	candidateHash,err:=registeredDigest("cote.csf.digest.commit-candidate",
		"cote.csf.schema.commit-candidate.body",candidate[:16])
	if err!=nil{return err}
	ref:=[]any{uint64(1),proof.source[:],proof.digest[:]}
	draft:=[]any{uint64(0),uint64(0),"physical.advanced",proof.completed,[]any{}}
	if expectedID!=domain.Digest(p.Candidate.Identity)||!equalBytes(candidate[0],expectedID[:])||
		!equalBytes(candidate[16],candidateHash[:])||
		!equalBytes(candidate[1],b.RunID[:])||!equalBytes(candidate[2],b.CycleID[:])||
		!sameCanonical(candidate[3],list)||candidate[4]!=proof.registration.ActionProducer||
		candidate[5]!=uint64(proof.registration.ActionVersion)||candidate[6]!=uint64(0)||
		candidate[7]!=uint64(0)||!sameCanonical(candidate[8],[]any{draft})||
		!sameCanonical(candidate[9],[]any{})||!sameCanonical(candidate[10],[]any{})||
		!sameCanonical(candidate[11],[]any{})||
		!sameCanonical(candidate[12],[]any{"physical.monotonic-hours"})||
		!sameCanonical(candidate[13],[]any{ref})||
		!sameCanonical(candidate[14],int64(0))||
		!sameCanonical(candidate[15],allowed["conflict-resolvers-v1"]){
		return errors.New("physical candidate has unregistered draft, invariant or source")
	}
	event,err:=settlement.PhysicalAdvancedEvent(domain.ID(b.RunID),domain.ID(b.CycleID),
		domain.ID(expectedID),proof.source,proof.digest,prior.Instant,prior.Revision,
		prior.NextLogicalSequence,proof.completed)
	if err!=nil{return err}
	if b.Events[0].Identity!=event.Identity||
		!bytes.Equal(b.Events[0].Canonical,event.Canonical)||
		!sameCanonical(journal[15],[]any{event.Identity[:]})||
		registeredInvisibleEngineEventFromCanonical(event.Canonical)!=nil{
		return errors.New("physical EventOrderKey, privacy or source does not match registered event")
	}
	dec,err:=strictFull(b.Decisions[0],16)
	if err!=nil{return err}
	decisionID,err:=registeredDigest("cote.csf.id.decision",
		"cote.csf.schema.decision-id-preimage",
		[]any{b.CycleID[:],journal[2],proof.source[:]})
	if err!=nil{return err}
	expectBody:=[]any{decisionID[:],b.RunID[:],b.CycleID[:],journal[2],
		ref,uint64(0),[]any{uint64(1),p.Candidate.Identity[:]},"physical.applied",
		[]any{},allowed["validators-v1"],allowed["conflict-resolvers-v1"],
		[]any{},[]any{event.Identity[:]},[]any{uint64(0)},[]any{uint64(0)}}
	dh,err:=registeredDigest("cote.csf.digest.decision-record",
		"cote.csf.schema.decision-record.body",expectBody)
	if err!=nil{return err}
	if b.Decisions[0].Identity!=[32]byte(decisionID)||
		!sameCanonical(dec[:15],expectBody)||!equalBytes(dec[15],dh[:]){
		return errors.New("physical DecisionRecord is not derived from registered candidate")
	}
	pairHash,err:=registeredDigest("cote.csf.digest.decision",
		"cote.csf.schema.decision-pair-list",
		[]any{[]any{proof.source[:],decisionID[:],dh[:]}})
	if err!=nil{return err}
	tasksHash,err:=registeredDigest("cote.csf.digest.perception-task-list",
		"cote.csf.schema.perception-task-id-list",[]any{})
	if err!=nil{return err}
	if !sameCanonical(journal[16],[]any{decisionID[:]})||
		!equalBytes(journal[17],pairHash[:])||!sameCanonical(journal[18],[]any{})||
		!equalBytes(journal[19],tasksHash[:]){
		return errors.New("physical CycleCommit decisions or no-disclosure task set invalid")
	}
	return nil
}

func registeredInvisibleEngineEventFromCanonical(raw []byte)error{
	decoded,err:=codec.StrictDecodePayload(raw)
	if err!=nil{return err}
	fields,ok:=decoded.([]any)
	if !ok{return errors.New("physical event not canonical")}
	return registeredInvisibleEngineEvent(fields)
}
