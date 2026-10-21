package sqlite

import (
    "bytes"
    "context"
    "database/sql"
    "errors"
    "fmt"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// CompleteInternalPerception handles ONLY the registered, secret engine
// lifecycle/synthetic events with zero observers. It deliberately has no
// argument for an observer list, output bytes, or worker-selected completion.
// A positive observation requires a future authenticated channel resolver.
// The persisted commit, event, task and genesis are reconciled under one
// SQLite transaction before the immutable completion is appended.
func (s *Store) CompleteInternalPerception(
    ctx context.Context, taskID [32]byte, bundle *policy.Bundle,
) (ports.CanonicalRecord, error) {
    var zero ports.CanonicalRecord
    if s == nil || bundle == nil { return zero, errors.New("perception requires store and bundle") }
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil { return zero, err }
    defer tx.Rollback()

    var taskRaw []byte
    if err := tx.QueryRowContext(ctx, "SELECT canonical FROM causal_outbox WHERE id=?", taskID[:]).Scan(&taskRaw); err != nil {
        return zero, fmt.Errorf("perception task not published: %w", err)
    }
    task := ports.CanonicalRecord{Identity: taskID, Canonical: taskRaw}
    taskValue, err := codec.StrictDecodePayload(taskRaw)
    if err != nil { return zero, err }
    fields, ok := taskValue.([]any)
    if !ok || len(fields) != 10 || !equalBytes(fields[0], taskID[:]) || fields[9] != uint64(0) {
        return zero, errors.New("perception task is not a canonical PENDING envelope")
    }
    run, runOK := idField(fields[1])
    cycle, cycleOK := idField(fields[2])
    eventID, eventOK := idField(fields[3])
    base, baseOK := fields[4].(uint64)
    result, resultOK := fields[5].(uint64)
    classifier, classOK := fields[6].([]any)
    resolver, resolverOK := fields[7].([]any)
    identityPolicy, identityOK := fields[8].([]any)
    if !runOK || !cycleOK || !eventOK || !baseOK || !resultOK ||
        !classOK || !resolverOK || !identityOK {
        return zero, errors.New("perception task authority fields malformed")
    }
    if len(resolver) != 3 || resolver[0] != "validators-v1" || resolver[1] != uint64(1) {
        return zero, errors.New("unregistered perception resolver")
    }

    var genesisRaw, eventRaw, journalRaw []byte
    if err := tx.QueryRowContext(ctx, "SELECT canonical FROM genesis_records WHERE run_id=?", run[:]).Scan(&genesisRaw); err != nil {
        return zero, fmt.Errorf("perception genesis missing: %w", err)
    }
    if _, err := genesis.ValidateCanonical(genesisRaw, bundle); err != nil {
        return zero, fmt.Errorf("perception genesis invalid: %w", err)
    }
    g, err := codec.StrictDecodePayload(genesisRaw)
    if err != nil { return zero, err }
    genesisFields, ok := g.([]any)
    if !ok || len(genesisFields) != 18 || !equalBytes(genesisFields[0], run[:]) {
        return zero, errors.New("perception genesis and task run disagree")
    }
    roster, ok := genesisFields[14].([]any)
    if !ok { return zero, errors.New("perception genesis policy roster absent") }
    refs := make([][]any, 0, len(roster))
    for _, item := range roster {
        ref, ok := item.([]any)
        if !ok { return zero, errors.New("malformed perception genesis policy") }
        refs = append(refs, ref)
    }

    if err := tx.QueryRowContext(ctx, "SELECT canonical FROM event_store WHERE id=?", eventID[:]).Scan(&eventRaw); err != nil {
        return zero, fmt.Errorf("perception source event missing: %w", err)
    }
    eventValue, err := codec.StrictDecodePayload(eventRaw)
    if err != nil { return zero, err }
    event, ok := eventValue.([]any)
    if !ok || len(event) != 19 { return zero, errors.New("invalid source Event shape") }
    // This allow-list is intentionally narrow. A resource or internal state
    // transition does not authorize an agent's knowledge even if the task's
    // potential-perceptibility predicate selected the event.
    kind, kindOK := event[3].(string)
    if !kindOK || (kind != "round.consumed" &&
        kind != "resource.consumed" &&
        kind != "clock.advanced") {
        return zero, errors.New("event requires a registered observer/channel resolver")
    }
    actors, ok := event[12].([]any)
    if !ok || len(actors) != 0 || event[15] != uint64(2) {
        return zero, errors.New("internal-only completion denied for addressable Event")
    }
    expected, err := perception.PlanPinned(refs, domain.ID(run), domain.ID(cycle),
        domain.WorldRevision(base), domain.WorldRevision(result),
        []ports.CanonicalRecord{{Identity: eventID, Canonical: eventRaw}},
        perception.Pins{Classification: classifier, Resolver: resolver, Identity: identityPolicy})
    if err != nil || len(expected) != 1 || expected[0].Identity != taskID ||
        !bytes.Equal(expected[0].Canonical, taskRaw) {
        return zero, errors.New("task does not match the genesis-pinned Event projection")
    }

    if err := tx.QueryRowContext(ctx, 
        "SELECT d.canonical FROM committed_cycles c JOIN decision_ledger d ON d.id=c.commit_id WHERE c.run_id=? AND c.cycle_id=?",
        run[:], cycle[:]).Scan(&journalRaw); err != nil {
        return zero, fmt.Errorf("perception source not committed: %w", err)
    }
    journalValue, err := codec.StrictDecodePayload(journalRaw)
    if err != nil { return zero, err }
    journal, ok := journalValue.([]any)
    if !ok || len(journal) != 24 || !equalBytes(journal[0], run[:]) ||
        !equalBytes(journal[1], cycle[:]) ||
        journal[6] != base || journal[8] != result ||
        !containsRoot(journal[15], eventID) || !containsRoot(journal[18], taskID) {
        return zero, errors.New("perception task/event not covered by committed CycleCommit")
    }
    // The current CSFRV1-5 has no authorized evidence write path. If any
    // evidence exists we must not assert an empty projection without a
    // complete, per-task evidence reconciliation (CSFRV1-6 next slice).
    var evidenceCount int
    if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM evidence_ledger").Scan(&evidenceCount); err != nil {
        return zero, err
    }
    if evidenceCount != 0 {
        return zero, errors.New("nonempty evidence ledger requires addressed resolver and task reconciliation")
    }
    completion, err := perception.EmptyCompletion(bundle, task)
    if err != nil { return zero, err }
    resultSQL, err := tx.ExecContext(ctx,
        "INSERT OR IGNORE INTO causal_outbox(id,canonical) VALUES(?,?)",
        completion.Identity[:], completion.Canonical)
    if err != nil { return zero, err }
    count, err := resultSQL.RowsAffected()
    if err != nil { return zero, err }
    if count == 0 {
        var prior []byte
        if err := tx.QueryRowContext(ctx, "SELECT canonical FROM causal_outbox WHERE id=?",
            completion.Identity[:]).Scan(&prior); err != nil { return zero, err }
        if !bytes.Equal(prior, completion.Canonical) {
            return zero, errors.New("perception completion identity collision")
        }
    }
    if err := tx.Commit(); err != nil { return zero, err }
    return completion, nil
}

func idField(value any) ([32]byte, bool) {
    var id [32]byte
    b, ok := value.([]byte)
    if !ok || len(b) != 32 { return id, false }
    copy(id[:], b)
    return id, true
}

func containsRoot(value any, id [32]byte) bool {
    records, ok := value.([]any)
    if !ok { return false }
    for _, item := range records {
        if equalBytes(item, id[:]) { return true }
    }
    return false
}

// CheckEmptyPerceptionBarrier is a fail-closed audit gate for the implemented
// zero-observer slice. It is NOT the final recipient-specific dispatch barrier.
// It checks every committed task at or before the requested revision. An
// unknown or positive-output completion cannot silently satisfy this gate.
func (s *Store) CheckEmptyPerceptionBarrier(
    ctx context.Context, run [32]byte, throughRevision uint64, bundle *policy.Bundle,
) error {
    if s == nil || bundle == nil { return errors.New("barrier requires store and bundle") }
    tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
    if err != nil { return err }
    defer tx.Rollback()

    rows, err := tx.QueryContext(ctx, "SELECT id,canonical FROM causal_outbox ORDER BY id")
    if err != nil { return err }
    var tasks []ports.CanonicalRecord
    for rows.Next() {
        var id, raw []byte
        if err := rows.Scan(&id, &raw); err != nil { rows.Close(); return err }
        record, err := codec.StrictDecodePayload(raw)
        if err != nil { rows.Close(); return err }
        fields, ok := record.([]any)
        if !ok { rows.Close(); return errors.New("invalid outbox record shape") }
        if len(fields) != 10 { continue } // completion or later outbox record
        key, keyOK := idField(id)
        if !keyOK || !equalBytes(fields[0], id) { rows.Close(); return errors.New("corrupt outbox task key") }
        if !equalBytes(fields[1], run[:]) { continue }
        revision, ok := fields[5].(uint64)
        if !ok { rows.Close(); return errors.New("corrupt outbox task revision") }
        if revision <= throughRevision {
            tasks = append(tasks, ports.CanonicalRecord{Identity:key, Canonical:raw})
        }
    }
    if err := rows.Err(); err != nil { rows.Close(); return err }
    if err := rows.Close(); err != nil { return err }
    for _, task := range tasks {
        expected, err := perception.EmptyCompletion(bundle, task)
        if err != nil { return err }
        var persisted []byte
        err = tx.QueryRowContext(ctx, "SELECT canonical FROM causal_outbox WHERE id=?",
            expected.Identity[:]).Scan(&persisted)
        if err == sql.ErrNoRows { return errors.New("perception barrier: prior task has no completion") }
        if err != nil { return err }
        if !bytes.Equal(persisted, expected.Canonical) {
            return errors.New("perception barrier: completion does not match zero-output projection")
        }
    }
    return tx.Commit()
}
