package commit

import (
	"bytes"
	"context"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
)

// frozenPhysicalCut uses ONLY already-durable, authenticated source authority.
// The original producer's memfd and private signing key are not needed after
// a crash; neither may be regenerated. The independent Genesis, InputLedger,
// source-hash and verified AdmissionFence must all agree. CommitPhysical then
// verifies them again under the sole SQLite CAS terminal writer.
func (c *Coordinator) frozenPhysicalCut(
	ctx context.Context, run domain.ID, sources []inputledger.StoredInput,
	entry []any,
) (embodimentv1.ApprovedCut, error) {
	var zero embodimentv1.ApprovedCut
	if c == nil || c.bundle == nil || c.store == nil || len(entry) != 5 || entry[1] != uint64(1) {
		return zero, errors.New("E2 invalid frozen physical source descriptor")
	}
	reg, err := c.bundle.PhysicalRegistration()
	if err != nil {
		return zero, err
	}
	sourceName, ok := entry[2].(string)
	if !ok || sourceName != reg.SourceID || reg.Disclosure != "ENGINE_ONLY" {
		return zero, errors.New("E2 frozen source not registered or not ENGINE_ONLY")
	}
	id, ok := entry[3].([]byte)
	digest, digestOK := entry[4].([]byte)
	if !ok || !digestOK || len(id) != 32 || len(digest) != 32 {
		return zero, errors.New("E2 frozen source ID/digest invalid")
	}
	var found *inputledger.StoredInput
	for i := range sources {
		if !bytes.Equal(sources[i].ID[:], id) {
			continue
		}
		if found != nil {
			return zero, errors.New("E2 duplicate frozen physical source identity")
		}
		found = &sources[i]
	}
	if found == nil || !bytes.Equal(found.Digest[:], digest) ||
		len(found.Fields) != 13 || found.Fields[2] != reg.SourceID ||
		found.Fields[4] != reg.ActionType || found.Fields[5] != uint64(reg.ActionVersion) {
		return zero, errors.New("E2 signed source not present in authenticated InputLedger")
	}
	raw, ok := found.Fields[7].([]any)
	if !ok || len(raw) != 5 || raw[0] != "physical.source-cut.v1" {
		return zero, errors.New("E2 persisted physical envelope malformed")
	}
	manifest, err := c.bundle.PhysicalManifestCanonical()
	if err != nil {
		return zero, err
	}
	registeredManifest, ok := raw[1].([]byte)
	cutRaw, cutOK := raw[2].([]byte)
	signature, signatureOK := raw[3].([]byte)
	completed, completedOK := raw[4].([]any)
	if !ok || !cutOK || !signatureOK || !completedOK ||
		!bytes.Equal(registeredManifest, manifest) {
		return zero, errors.New("E2 installed physical manifest does not match staged source")
	}
	genesis, err := c.store.ValidatedGenesis(ctx, [32]byte(run), c.bundle)
	if err != nil || genesis.RunID != run {
		return zero, errors.New("E2 Genesis no longer authenticates the frozen source")
	}
	recovered, err := embodimentv1.RecoverApprovedCutFromDurableIngress(
		cutRaw, signature, completed, reg, run, genesis.Digest)
	if err != nil || !recovered.IsDurablyRecovered() {
		return zero, errors.New("E2 frozen physical source is not a valid signed completed cut")
	}
	return recovered, nil
}
