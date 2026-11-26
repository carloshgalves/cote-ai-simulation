# Annual qualification runbook

New V26+ save; configure routes first. Stop the UI process before CLI use; preserve the same save header and sidecar directory.

- Real provider benchmark to first exam: go run ./cmd/cote-local -provider ui -save cote-v26-benchmark.json -until first-exam > first-exam.json
- Continue through first result: same command/save, -until first-result > first-result.json
- Full year (explicit potentially high provider cost): same save, -until year-end > year-end.json
- No inference audit of existing committed history: same save, -audit-only > audit.json

The commands measure actual wall clock and provider attempt deltas for that invocation; no extrapolated speed claimed. If interrupted or a provider fails, the save persists confirmed segments, output describes partial progress, exit 2. Audit verifies exact replay, grade participation partition, results, named/generic submissions, body endpoints, dialogue and social counts. Structural gate alone is not fidelity acceptance. Independent repeated model trajectories and blind character evaluation are required; transition_allowed deliberately false until accepted. Full year has NOT been executed in the connector environment.

Private optional candidate manifest is a local mode-0600 JSON file with schema private-admissions-v1 and candidates: id (oc-*), name, admission_year:2, class A/B/C/D/auto, traits, goals, background, source. CLI -admissions <private-path> validates uniqueness and schema without importing or logging content. No automatic promotion or new admission. Existing student state, chronology, class economics, admissions and cohort reconciliation must be implemented/validated before activation of Year 2. Do not commit private manifests. 
