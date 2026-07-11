# Story 1.2: Establish Persistent Audit Log Pipeline

Status: ready-for-dev

## Story

As a security operator,
I want all API audit events to have a stable, validated schema and be shippable to persistent centralized storage,
so that I can reconstruct any incident with confidence.

## Acceptance Criteria

1. Audit log output is valid structured JSON on every line (one JSON object per log entry)
2. Required fields present on every audit entry: `principal`, `remote`, `method`, `path`, `device`, `command`, `result`, `request_id`, `timestamp`
3. Unit test asserts audit log JSON schema — fails build if any required field is missing or renamed
4. Helm chart includes annotations and documentation for log shipper integration (Fluentd/Fluent Bit sidecar pattern)
5. Runbook documents log retention policy (minimum 90 days recommended)
6. Sensitive fields (`token`, `password`, raw `Authorization` header value) confirmed absent from all log output (negative test)

## Tasks / Subtasks

- [ ] Task 1: Audit log schema stabilization (AC: 1, 2)
  - [ ] Review current audit log output in `internal/api/handlers.go`
  - [ ] Ensure all required fields are always present (use zero/empty values when not applicable, not omit)
  - [ ] Add `timestamp` field (RFC 3339) if not already present
  - [ ] Define `AuditEntry` struct to enforce schema at compile time

- [ ] Task 2: Schema regression tests (AC: 3, 6)
  - [ ] Unit test: capture log output for each endpoint; unmarshal JSON; assert all required fields present and non-empty
  - [ ] Negative test: assert `Authorization`, `token`, `password` substrings do NOT appear anywhere in log output
  - [ ] Test covers: success path, auth failure, validation failure, scope failure, downstream failure

- [ ] Task 3: Helm log shipper documentation (AC: 4)
  - [ ] Add `logging` block to `helm/clock-server/values.yaml` with example Fluent Bit sidecar config (commented out)
  - [ ] Add pod annotation `fluentbit.io/parser: json` (optional, off by default)
  - [ ] Document in chart README how to enable log shipping

- [ ] Task 4: Runbook (AC: 5)
  - [ ] Add `docs/runbooks/audit-log-retention.md`
  - [ ] Document recommended retention (90 days), example Loki/ELK index configuration, and how to query for a specific `request_id`

## Dev Notes

- Current audit logging is in `internal/api/handlers.go` — look for `log.Printf` or structured log calls
- If using `log/slog` (Go 1.21+), output is already structured JSON with `slog.New(slog.NewJSONHandler(os.Stderr, nil))` — confirm this is the current implementation or migrate to it
- `AuditEntry` struct should be in `internal/api/` package; use `encoding/json` marshal in tests to validate output
- For the negative test: capture `os.Stderr` output during handler execution and scan for sensitive substrings
- Do NOT log the raw token value — log only the credential `id` (the `principal` field)

### Project Structure Notes

- Modified: `internal/api/handlers.go` — audit log field alignment
- New: `internal/api/audit.go` — `AuditEntry` struct (if not already extracted)
- Modified/New: `internal/api/handlers_test.go` — schema tests
- New: `docs/runbooks/audit-log-retention.md`
- Modified: `helm/clock-server/values.yaml`

### References

- [Source: internal/api/handlers.go] — Current audit logging implementation
- [Source: internal/security/auth.go] — `Credential.id` field (safe to log; token is not)

## Dev Agent Record

### Agent Model Used

claude-sonnet-4-6

### Debug Log References

### Completion Notes List

### File List

- `internal/api/audit.go` (new) — `AuditEntry` struct, `writeAuditEntry`, package-level `auditWriter`, and `(*Handler).audit` method
- `internal/api/handlers.go` (modified) — removed old `log.Printf` audit method and `log` import; audit calls now delegate to `audit.go`
- `internal/api/audit_test.go` (new) — schema regression tests, negative sensitive-data tests, compile-time struct contract test
- `helm/clock-server/values.yaml` (modified) — added `logging` block with Fluent Bit sidecar documentation (commented out, `enabled: false`)
- `docs/runbooks/audit-log-retention.md` (new) — retention policy, log shipping options (Fluent Bit sidecar, DaemonSet, Elasticsearch), example queries, alerting recommendations
