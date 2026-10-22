package commit

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// ResumeRegisteredWorkV1 closes the loop from a verified snapshot to an
// authoritative terminal. The admission fence, NOT a caller-chosen handler,
// determines which already-registered V1 executor can run. Unsupported mixed
// cohorts remain sealed and fail closed without a second publication path.
func (c *Coordinator) ResumeRegisteredWorkV1(ctx context.Context, ledger *inputledger.Ledger,
 run, snapshot [32]byte) (ResumeWorkResult, error) {
 return c.ResumeRunV1(ctx, ledger, run, snapshot, c.DispatchFencedWorkV1)
}

// DispatchFencedWorkV1 serves both uninterrupted V1 work and crash recovery.
// It never admits new units or chooses a cycle: the strictly verified terminal
// handler below independently checks the persisted fence and its complete cut.
func (c *Coordinator) DispatchFencedWorkV1(ctx context.Context, ledger *inputledger.Ledger,
 cycle admission.Cycle, fence admission.Fence) (ports.CanonicalRecord, error) {
 var empty ports.CanonicalRecord
 if c == nil || c.store == nil || c.bundle == nil || ledger == nil ||
  !ledger.SharesFenceAuthority(c.store) {
  return empty, errors.New("registered dispatcher requires shared persisted authority")
 }
 view, _, err := ledger.AdmissionView(ctx, c.bundle)
 if err != nil { return empty, err }
 if err := admission.VerifyFence(c.bundle, view, cycle, c.admissionPolicies, fence); err != nil {
  return empty, err
 }
 persisted, err := c.store.GetFence(ctx, [32]byte(fence.ID))
 if err != nil || !bytes.Equal(persisted, fence.Canonical) {
  return empty, errors.New("registered dispatcher refuses an unsealed or altered fence")
 }
 units, ok := fence.Fields[11].([]any)
 if !ok || len(units) == 0 { return empty, errors.New("empty or malformed V1 WORK fence") }
 _, _, _, _, byRound, run := view.Snapshot()
 responses := map[domain.ID]inputledger.Response{}
 for _, slots := range byRound {
  for _, response := range slots {
   if _, duplicate := responses[response.ID]; duplicate {
    return empty, errors.New("duplicated response identity in admitted cohort")
   }
   responses[response.ID] = response
  }
 }
 loader, ok := c.store.(interface {
  LoadSystemUnit(context.Context, [32]byte, [32]byte) ([]byte, uint8, error)
 })
 if !ok { return empty, errors.New("registered dispatcher lacks system unit authority") }
 var resources, noProposals, sends, system int
 var delivery bool
 for _, raw := range units {
  entry, ok := raw.([]any)
  if !ok || len(entry) != 5 { return empty, errors.New("malformed fenced source") }
  kind, ok := entry[1].(uint64)
  if !ok { return empty, errors.New("unregistered fenced source kind") }
  idRaw, validID := entry[3].([]byte)
  digestRaw, validDigest := entry[4].([]byte)
  if !validID || !validDigest || len(idRaw) != 32 || len(digestRaw) != 32 {
   return empty, errors.New("fenced source has invalid canonical identity or digest")
  }
  var id domain.ID
  copy(id[:], idRaw)
  switch kind {
  case 0:
   response, found := responses[id]
   if !found || !bytes.Equal(response.Digest[:], digestRaw) ||
    response.CycleID != cycle.ID || response.RunID != run {
    return empty, errors.New("fenced slot response is missing or unauthenticated")
   }
   if response.Kind == 1 {
    noProposals++
    continue
   }
   if response.Kind != 0 || len(response.Body) != 18 {
    return empty, errors.New("unknown registered slot response")
   }
   action, ok := response.Body[4].(string)
   version, versionOK := response.Body[5].(uint64)
   if !ok || !versionOK || version != 1 {
    return empty, errors.New("unregistered action schema")
   }
   switch action {
   case "resource.consume":
    resources++
   case "communication.send":
    sends++
   default:
    return empty, fmt.Errorf("no V1 terminal registered for action %q", action)
   }
  case 2, 3:
   sourceRaw, actualKind, err := loader.LoadSystemUnit(ctx, [32]byte(run), [32]byte(id))
   if err != nil || uint64(actualKind) != kind {
    return empty, errors.New("system unit differs from persisted authority")
   }
   value, err := codec.StrictDecodePayload(sourceRaw)
   if err != nil { return empty, err }
   fields, ok := value.([]any)
   if !ok || len(fields) < 12 {
    return empty, errors.New("invalid registered system source")
   }
   first, firstOK := fields[0].([]byte)
   final, finalOK := fields[len(fields)-1].([]byte)
   if !firstOK || !finalOK || !bytes.Equal(first, idRaw) ||
    !bytes.Equal(final, digestRaw) || fields[2] != entry[2] {
    return empty, errors.New("system source differs from frozen fence")
   }
   if kind == 2 && fields[5] == "communication.delivery" { delivery = true }
   system++
  default:
   return empty, errors.New("no registered executor for fenced source kind")
  }
 }
 switch {
 case sends == 1 && resources == 0 && noProposals == 0 && system == 0 && len(units) == 1:
  return c.CommitCommunicationSend(ctx, ledger, cycle, fence)
 case sends > 0:
  return empty, errors.New("communication send cannot be silently mixed with another V1 source")
 case delivery && system == 1 && resources == 0 && noProposals == 0 && len(units) == 1:
  return c.CommitCommunicationDue(ctx, ledger, cycle, fence)
 case delivery:
  return empty, errors.New("delivery occurrence requires its own one-source V1 terminal")
 case resources == 2 && noProposals == 0:
  return c.CommitTwoResourceContest(ctx, ledger, cycle, fence)
 case resources == 1 && noProposals == 0:
  return c.CommitSingleResource(ctx, ledger, cycle, fence)
 case resources > 0:
  return empty, errors.New("unregistered resource/NoProposal or multi-contest cohort")
 case noProposals > 0:
  return c.CommitNoProposal(ctx, ledger, cycle, fence)
 case system > 0:
  return c.CommitSystemUnit(ctx, ledger, cycle, fence)
 default:
  return empty, errors.New("fence does not match a registered V1 terminal")
 }
}
