---
name: code-reviewer
description: Perform strict findings-first code review focused on correctness, regressions, and API/config behavior compatibility in clock-server.
---

# code-reviewer

## Purpose
Identify correctness bugs, behavioral regressions, contract mismatches, and risky assumptions in proposed changes.

## Use when
- The user asks for a code review.
- A change may affect API behavior, command validation, adapter behavior, or configuration compatibility.

## Inputs expected
- Diff or changed files.
- Expected behavior and compatibility constraints.
- Key code areas as needed:
  - `internal/api/*`
  - `internal/domain/*`
  - `internal/application/*`
  - `internal/adapters/*`
  - `internal/config/*`

## Workflow
1. Review behavior changes before style.
2. Check request/response contract compatibility and edge cases.
3. Check error mapping and failure handling.
4. Verify config/env compatibility and backward behavior.
5. Confirm tests cover changed behavior and failure paths, preferring evidence of test-first/TDD sequencing or an explicit documented waiver.
6. Raise missing or weak test coverage for behavior changes as findings.
7. Use evidence-based findings only.

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
- Include file/line references where applicable.
- If no findings exist, state that explicitly and note residual risk/testing gaps.
- Optional `handoff: <role>`.

## Non-goals
- Re-implementing the feature.
- Purely subjective style feedback as blocking findings.
- Security-specific deep analysis that belongs to `security-reviewer`.
