package sqlite

import (
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// ForkIdleSnapshotV1 starts a new run from a causally verified checkpoint in
// a distinct SQLite workspace. Its bounded V1 migrators reconstruct pending
// Genesis-owned occurrences, initial activations, and UNSTARTED Genesis-owned
// rounds, deriving child identities and sealing all records atomically.
// Dispatches (including revoked), slot responses, Event/Claim-owned work,
// in-transit communication, evolved trigger runtime and cognition checkpoints
// require separate versioned migrators and fail closed; none may be dropped.
// The immutable parental prefix remains archived, never child ledger authority.
func (s *Store) ForkIdleSnapshotV1(ctx context.Context,sourceRun,parentSnapshotID,childRun [32]byte,
 child *Store,b *policy.Bundle)(*genesis.Genesis,error){
 return s.forkSnapshotWithInstalledPolicies(ctx,sourceRun,parentSnapshotID,childRun,child,b,b,false)
}

// ForkIdleSnapshotV1ToV2 is the positive cross-policy bootstrap for verified
// IDLE cuts supported by the registered causal migrators. Parent V1 history is preserved
// byte-for-byte, while child Genesis, clock and future authorities are V2.
// Pending communication, Event/Genesis occurrences and round history are
// authenticated migration births. V2 also imports snapshot-time trigger seeds
// and their pending activation proofs; unsupported source classes fail closed.
func (s *Store) ForkIdleSnapshotV1ToV2(ctx context.Context,sourceRun,parentSnapshotID,childRun [32]byte,
 child *Store,origin,version2 *policy.Bundle)(*genesis.Genesis,error){
 if origin==nil||version2==nil{return nil,errors.New("cross-policy fork requires both installed bundles")}
 id,v,_,_,err:=origin.CodecIdentity()
 if err!=nil||id!=policy.CodecPolicyID||v!=1{return nil,errors.New("cross-policy fork origin must be installed V1")}
 id,v,_,_,err=version2.CodecIdentity()
 if err!=nil||id!=policy.CodecPolicyV2ID||v!=2{return nil,errors.New("cross-policy fork child must be installed V2")}
 historic,err:=version2.ResolveHistoricalOrigin(policy.CodecPolicyID,origin.CodecPolicyHash)
 if err!=nil||historic==version2{return nil,errors.New("V2 lacks exactly pinned V1 origin")}
 return s.forkSnapshotWithInstalledPolicies(ctx,sourceRun,parentSnapshotID,childRun,child,origin,version2,true)
}

func (s *Store) forkSnapshotWithInstalledPolicies(ctx context.Context,sourceRun,parentSnapshotID,childRun [32]byte,
 child *Store,origin,childPolicy *policy.Bundle,crossPolicy bool)(*genesis.Genesis,error){
 cut,err:=s.prepareVerifiedForkCut(ctx,sourceRun,parentSnapshotID,childRun,child,origin,childPolicy,crossPolicy)
 if err!=nil{return nil,err}
 snapshot:=cut.snapshot
 draft,err:=s.prepareForkMigrationDraft(ctx,sourceRun,parentSnapshotID,childRun,cut,origin,childPolicy,crossPolicy)
 if err!=nil{return nil,err}
 // Re-verify and copy the exact parent ledger prefixes at the ORIGINAL
 // read-transaction boundary. Until it succeeds, only a draft exists:
 // no complete plan may be supplied to the child SQL publisher.
 archived,err:=s.verifiedForkArchive(ctx,parentSnapshotID,snapshot,origin)
 if err!=nil{return nil,err}
 plan:=forkMigrationPlan{forkMigrationDraft:draft,archived:archived}
 if err:=child.publishForkChildTransactional(ctx,s,sourceRun,parentSnapshotID,childRun,cut,plan,origin,childPolicy,crossPolicy);err!=nil{
  return nil,err
 }
 return plan.childGenesis,nil
}

// Compile-time guarantees for the run identity and the parent/child
// responsibility boundary; no shared world truth authority is imported.
var _ domain.ID = domain.ID{}
