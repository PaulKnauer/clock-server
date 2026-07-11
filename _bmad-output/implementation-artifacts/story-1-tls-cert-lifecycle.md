# Story 1.1: Automate TLS Certificate Lifecycle with cert-manager

Status: completed

## Story

As an operator,
I want TLS certificates to be automatically renewed via cert-manager,
so that certificate expiry never causes a production outage.

## Acceptance Criteria

1. Helm chart includes optional `Certificate` and `Issuer` resource templates for cert-manager integration
2. `values.yaml` supports `tls.certManager.enabled`, `tls.certManager.issuerRef`, `tls.certManager.duration`, `tls.certManager.renewBefore`
3. Server startup validation rejects an already-expired TLS certificate and exits non-zero
4. Runbook documents manual cert rotation fallback procedure
5. Integration test: server starts successfully with a valid cert; refuses to start with an expired cert
6. Alert threshold documented: warn when cert expiry < 30 days

## Tasks / Subtasks

- [x] Task 1: Add cert-manager Helm templates (AC: 1, 2)
  - [x] Create `helm/clock-server/templates/certificate.yaml` (guarded by `tls.certManager.enabled`)
  - [x] Add `tls.certManager` block to `helm/clock-server/values.yaml` with sensible defaults
  - [x] Helm lint passes with certManager enabled and disabled

- [x] Task 2: Server startup cert expiry validation (AC: 3)
  - [x] In `cmd/server/main.go` or `internal/config/config.go`, load and parse the TLS cert at startup
  - [x] If cert `NotAfter` is in the past, log error and `os.Exit(1)`
  - [x] If cert expires within 30 days, log a warning (non-fatal)

- [x] Task 3: Tests (AC: 5)
  - [x] Unit test in `internal/config` or `cmd/server`: expired cert → startup error
  - [x] Unit test: valid cert → no error
  - [x] Unit test: cert expiring in 15 days → warning logged

- [x] Task 4: Runbook documentation (AC: 4, 6)
  - [x] Add `docs/runbooks/tls-cert-rotation.md` with manual rotation steps
  - [x] Document cert-manager automatic renewal flow
  - [x] Document the 30-day alert threshold and recommended action

## Dev Notes

- TLS cert is currently loaded in `cmd/server/main.go` via `tls.LoadX509KeyPair`; expiry check should happen right after this call
- `x509.Certificate.NotAfter` gives the expiry time — compare against `time.Now()`
- cert-manager `Certificate` resource spec reference: https://cert-manager.io/docs/reference/api-docs/#cert-manager.io/v1.Certificate
- Helm templates should be guarded with `{{- if .Values.tls.certManager.enabled }}` to remain opt-in
- Do NOT break existing `TLS_CERT_FILE` / `TLS_KEY_FILE` env var flow — cert-manager just manages the files that get mounted

### Project Structure Notes

- New Helm templates: `helm/clock-server/templates/certificate.yaml`
- Modified: `helm/clock-server/values.yaml`, `cmd/server/main.go`
- New test file or additions to: `cmd/server/main_test.go`
- New runbook: `docs/runbooks/tls-cert-rotation.md`

### References

- [Source: cmd/server/main.go] — TLS loading and server bootstrap
- [Source: internal/config/config.go] — Config validation pattern to follow
- [Source: helm/clock-server/values.yaml] — Existing TLS values block

## Dev Agent Record

### Agent Model Used

claude-sonnet-4-6

### Debug Log References

None.

### Completion Notes List

- `checkCertExpiry` extracted as a pure function with injected `time.Time` for full unit-test coverage without mocking `os.Exit`.
- TLS cert check runs only when both `TLS_CERT_FILE` and `TLS_KEY_FILE` are set; TRUST_PROXY_TLS-only deployments are unaffected.
- cert-manager `Certificate` template is fully guarded by `{{- if .Values.tls.certManager.enabled }}` and defaults to `false`.
- All existing tests continue to pass; `go test -race ./cmd/server/... ./internal/config/...` green.

### File List

**Modified:**
- `cmd/server/main.go` — added `checkCertExpiry` function and startup cert expiry validation
- `cmd/server/main_test.go` — added unit tests for expired/warning/valid cert scenarios
- `helm/clock-server/values.yaml` — added `tls.certManager` block

**Created:**
- `helm/clock-server/templates/certificate.yaml` — cert-manager `Certificate` resource template
- `docs/runbooks/tls-cert-rotation.md` — TLS cert rotation runbook
