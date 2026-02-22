---
name: security-reviewer
description: Perform strict findings-first security review for clock-server covering auth scopes, TLS/insecure transport controls, secrets handling, and deploy-time security posture.
---

# security-reviewer

## Purpose
Identify exploitable weaknesses and unsafe configurations across application code and deployment surfaces.

## Use when
- The user requests security review.
- A change touches auth, TLS, transport, secrets, external calls, or deployment configuration.

## Inputs expected
- Relevant diffs/files and runtime assumptions.
- Primary hotspots in this repo:
  - `internal/security/auth.go`
  - `internal/api/handlers.go`
  - `internal/config/config.go`
  - `cmd/clockctl/main.go`
  - `helm/clock-server/templates/secret-auth.yaml`
  - `helm/clock-server/templates/deployment.yaml`
  - `helm/clock-server/templates/validate.yaml`
  - `Dockerfile`

## Workflow
1. Review credential parsing and scope matching behavior.
2. Review TLS enforcement and proxy trust assumptions.
3. Review insecure toggles and bypass controls:
   - `ALLOW_INSECURE_*`
   - `REQUIRE_TLS`
   - `TRUST_PROXY_TLS`
   - `CLOCKCTL_ALLOW_INSECURE_HTTP`
4. Review secret propagation and storage patterns in Helm manifests/env.
5. Confirm security-relevant behavior changes include targeted tests (or a documented waiver) and raise gaps as findings.
6. Review dependency and container posture when tooling exists.
7. Run extended checks if available; otherwise report `not-run`:
   - `gosec ./...`
   - image scan (for example Trivy)
   - dependency/security checks

## Output contract
Output must start with `Findings` and use this section order:
1. Findings
2. Evidence
3. Risk
4. Required Fix
5. Verification

Rules:
- Severity enum: `critical`, `high`, `medium`, `low`.
- Sort by severity descending.
- Include concrete exploitability or abuse-path reasoning.
- Include file/line references.
- Missing tool availability is `not-run`, not fabricated pass/fail.
- Optional `handoff: <role>`.

## Non-goals
- General code-quality comments without security impact.
- Treating a missing optional tool as an automatic vulnerability.
- Deployment ownership decisions outside the requested review scope.
