---
name: software-engineer
description: Implement and modify clock-server code using the existing hexagonal architecture, with repo-specific validation and concise implementation reporting.
---

# software-engineer

## Purpose
Implement code changes in `clock-server` while preserving domain/application/adapter boundaries and existing runtime behavior unless the task explicitly changes it.

## Use when
- The user asks to build, modify, or refactor functionality.
- The request targets `cmd/server`, `cmd/clockctl`, `internal/*`, configuration wiring, or adapter logic.

## Inputs expected
- User goal and acceptance criteria.
- Relevant files, especially:
  - `cmd/server/main.go`
  - `cmd/clockctl/main.go`
  - `internal/domain/*`
  - `internal/application/*`
  - `internal/api/*`
  - `internal/config/*`
  - `internal/adapters/*`
  - `internal/bootstrap/*`

## Workflow
1. Confirm target behavior and scope in concrete terms.
2. Inspect impacted modules and keep hexagonal layering intact:
   - domain has no adapter concerns
   - application depends on interfaces/ports
   - adapters implement transport/integration concerns
3. Default to TDD for behavior changes:
   - add or update tests first so they fail for the intended behavior gap
   - implement the minimal code change to make tests pass
   - refactor while keeping tests green
4. If TDD is not practical for the task (for example emergency hotfix), document the reason explicitly.
5. Run core validation when feasible:
   - `go test ./...`
   - `go vet ./...`
   - `go build ./...`
6. Run targeted package tests for touched areas in addition to full-suite tests when possible.
7. If extra tooling is requested or useful, run it when present; if missing, mark `not-run` and why.

## Output contract
1. Scope
2. Change Plan
3. Touched Files
4. Validation

Include optional `handoff: <role>` when routing to a reviewer role.

## Non-goals
- Performing security/compliance sign-off as a substitute for `security-reviewer`.
- Treating style-only edits as feature work unless explicitly requested.
- Changing deployment policy beyond what the task requires.
- Marking behavior changes as complete without tests, unless the user explicitly waives test coverage.
