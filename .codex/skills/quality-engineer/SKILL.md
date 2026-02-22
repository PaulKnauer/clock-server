---
name: quality-engineer
description: Perform strict findings-first quality review of tests, coverage intent, reliability, and verification depth for clock-server changes.
---

# quality-engineer

## Purpose
Ensure changes are validated by sufficient tests and reliability checks, with explicit gaps documented as findings.

## Use when
- The user requests QA/test review.
- A change affects behavior and needs confidence on correctness and regressions.

## Inputs expected
- Changed files and intended behavior.
- Existing tests, especially:
  - `internal/*/*_test.go`
  - `cmd/clockctl/main_test.go`
  - `internal/adapters/mqtt/fuzz_test.go`

## Workflow
1. Map behavior changes to required test scenarios.
2. Verify tests are written first for behavior changes by default (TDD), or ensure there is an explicit waiver with rationale.
3. Check happy path, edge cases, and error-path coverage.
4. Verify command/input validation paths for API/CLI.
5. Run core checks when feasible:
   - `go test ./...`
   - `go vet ./...`
   - `go build ./...`
6. Run optional reliability checks when feasible:
   - `go test -race ./...`
   - focused package-level tests
7. If a tool/check is unavailable or too expensive in context, report `not-run` with reason.

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
- Missing tests for changed behavior are explicit findings.
- Include file/line references where applicable.
- Optional `handoff: <role>`.

## Non-goals
- Replacing implementation work.
- Security analysis beyond testability/reliability impact.
- Blocking on non-material style-only concerns.
