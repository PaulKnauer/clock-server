# Story 1.5: Zero-Downtime Credential Rotation

Status: completed

## Story

As an operator,
I want to rotate API credentials without restarting the service,
so that credential rotation is low-risk, non-disruptive, and happens frequently.

## Acceptance Criteria

1. Server reloads `API_AUTH_CREDENTIALS` on `SIGHUP` signal (hot reload)
2. After reload: new credential is accepted, rotated-out credential is rejected — without a process restart
3. Credential reload is atomic: there is no window where zero credentials are valid
4. Reload errors are logged and do NOT replace the currently active credential set (fail-safe: keep old credentials on parse error)
5. Unit test: credential set updated in-place; old token rejected, new token accepted after reload
6. Runbook documents the rotation procedure: update K8s Secret → trigger reload (SIGHUP or rolling restart)
7. `helm/clock-server/values.yaml` documents recommended rotation frequency

## Tasks / Subtasks

- [x] Task 1: Implement SIGHUP credential reload (AC: 1, 2, 3, 4)
  - [x] In `cmd/server/main.go`, register `signal.Notify(sigCh, syscall.SIGHUP)`
  - [x] On SIGHUP: re-read `API_AUTH_CREDENTIALS` env var (or re-read from file if path-based)
  - [x] Parse new credentials; if parse fails → log error, keep existing credentials, do NOT swap
  - [x] If parse succeeds → atomically swap credential store (use `sync/atomic` pointer swap or mutex-guarded replace on the auth store)
  - [x] Log successful reload with count of credentials loaded (not the credential values)

- [x] Task 2: Credential store refactor for atomic swap (AC: 3)
  - [x] Ensure `internal/security/auth.go` credential store supports atomic replacement
  - [x] If credentials are currently a plain slice/map, wrap in a struct with `sync.RWMutex` or use `atomic.Pointer[[]Credential]`
  - [x] All request-path reads use read lock; reload uses write lock (or atomic pointer swap)

- [x] Task 3: Tests (AC: 5)
  - [x] Unit test: start with credential A; trigger reload with credential B; assert A rejected, B accepted
  - [x] Unit test: trigger reload with malformed credentials; assert A still accepted (fail-safe)
  - [x] Unit test: concurrent requests during reload do not panic (race detector must pass)

- [x] Task 4: Runbook and documentation (AC: 6, 7)
  - [x] Add `docs/runbooks/credential-rotation.md` with step-by-step procedure
  - [x] Document K8s rolling restart as alternative if SIGHUP is not accessible
  - [x] Add comment in `helm/clock-server/values.yaml` recommending 90-day rotation maximum

## Dev Notes

- `syscall.SIGHUP` is the conventional Unix signal for "reload config" — well understood by ops tooling
- Atomic pointer swap pattern: `var store atomic.Pointer[[]security.Credential]`; `store.Store(&newCreds)` is a single atomic operation with no lock contention window
- In Kubernetes: `kubectl exec <pod> -- kill -HUP 1` sends SIGHUP to PID 1 in the container; document this in the runbook
- Alternative: rolling restart via `kubectl rollout restart deployment/clock-server` — simpler but involves brief dual-version window; document both
- Do NOT log credential values at any point during reload — log only the credential `id` fields and the count
- The race detector test for concurrent requests during reload is critical — this is the most likely race condition

### Project Structure Notes

- Modified: `cmd/server/main.go` — SIGHUP signal handler
- Modified: `internal/security/auth.go` — atomic credential store
- New: `docs/runbooks/credential-rotation.md`
- Modified: `helm/clock-server/values.yaml` — rotation frequency comment

### References

- [Source: internal/security/auth.go] — Current `Credential` struct and `ParseCredentials()`
- [Source: cmd/server/main.go] — Signal handling and shutdown pattern (model SIGHUP after existing SIGTERM handling)
- [Source: internal/config/config.go] — `API_AUTH_CREDENTIALS` env var parsing

## Dev Agent Record

### Agent Model Used

claude-sonnet-4-6

### Debug Log References

### Completion Notes List

### File List

- `internal/security/auth.go` — `CredentialStore` with `atomic.Pointer[[]Credential]`, `NewCredentialStore`, `Load`, `Replace`
- `internal/security/auth_test.go` — added `TestCredentialStoreReload`, `TestCredentialStoreFailSafeOnBadReload`, `TestCredentialStoreConcurrentReplace`
- `cmd/server/main.go` — SIGHUP goroutine: re-reads env, parses, atomically replaces or logs error (fail-safe)
- `internal/config/config.go` — `Config.AuthCredentials` is `*security.CredentialStore`
- `internal/api/handlers.go` — `Handler.credentials` is `*security.CredentialStore`, `lookupCredential` calls `credentials.Load()`
- `internal/api/audit_test.go` — updated to use `security.NewCredentialStore(...)`
- `internal/api/bdd_test.go` — updated `credentials` field type and initialisation to `*security.CredentialStore`
- `internal/api/handlers_test.go` — updated to use `security.NewCredentialStore(...)`
- `internal/api/metrics_test.go` — updated to use `security.NewCredentialStore(...)`
- `internal/config/config_test.go` — updated `TestLoadFromEnvParsesCredentialScopes` to call `.Load()`
- `docs/runbooks/credential-rotation.md` — new runbook: SIGHUP procedure, rolling restart alternative, verification steps, 90-day recommendation
- `helm/clock-server/values.yaml` — added credentials rotation policy comment with 90-day max interval
