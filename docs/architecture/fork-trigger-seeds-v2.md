# R21 — trigger seeds at a V1 → V2 boundary

The exact verified parent snapshot supplies the closed trigger-runtime and
pending-activation rosters. The atomic V2 bootstrap archives the source registry
seal, canonical definitions and pending activation births in a versioned trigger
import manifest. The source registry is verified before copying; its definition
digests and Genesis binding are rechecked on lineage verification. These are
SQLite migration proofs, not a change to the frozen V1 policy or wire schemas.

The child keeps definition bytes and predicate policy. It keeps last predicate,
arming, exhaustion, activation count and absolute repeat instant from the cut;
only evaluated-through revision is rebased to zero. A pending activation keeps
its count and predicate-result digest, authenticates its exact source ID/digest
and Genesis or committed cycle origin, and is reminted under the child Genesis.
Same-instant eligibility is rebased relative to the parent's next ordinal.
No trigger is evaluated again at birth and no event or knowledge is fabricated.

Lineage verification rederives seeds and the entire Genesis-owned activation
roster from the immutable import manifest plus archived source checkpoint and
five ledgers. Replay starts from those seeds and checks current runtime only
after folding the child's own journal. Missing/changed proofs or extra births
fail closed. Later child activations retain their ordinary cycle provenance.
Existing V2 initial-state forks without a trigger manifest retain the existing
Genesis evaluator; they cannot import evolved state through that compatibility
path. Same-policy V1 forks retain their existing fail-closed boundary.

R21's broader cascade/failure matrix and R22/R23 acceptance gates remain separate.
