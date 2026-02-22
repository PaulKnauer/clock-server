## Skills
This repository defines role-specific skills in `.codex/skills/*`.

### Available roles
- software-engineer: implement and modify code safely in this repo.
- code-reviewer: review correctness, regressions, and API behavior.
- security-reviewer: review auth, transport, secrets, and dependency/security posture.
- quality-engineer: review test quality, coverage intent, and reliability checks.
- devops-engineer: review Docker/Compose/Helm/CI/release/deployment readiness.

### Role location
- `software-engineer`: `.codex/skills/software-engineer/SKILL.md`
- `code-reviewer`: `.codex/skills/code-reviewer/SKILL.md`
- `security-reviewer`: `.codex/skills/security-reviewer/SKILL.md`
- `quality-engineer`: `.codex/skills/quality-engineer/SKILL.md`
- `devops-engineer`: `.codex/skills/devops-engineer/SKILL.md`

### Trigger rules
- A role is selected when explicitly named as either:
  - `$software-engineer`, `$code-reviewer`, `$security-reviewer`, `$quality-engineer`, `$devops-engineer`
  - or plain text aliases: "software engineer", "code reviewer", "security reviewer", "quality engineer", "devops engineer"
- Use only explicitly named roles unless the user asks for a multi-role pass.
- Roles do not persist across turns unless re-named in a later prompt.

### Orchestration
- On-demand only: run the role(s) the user names.
- No mandatory chaining.
- Optional manual handoff marker in outputs: `handoff: <role>`.

### Output contracts
- Reviewer roles (`code-reviewer`, `security-reviewer`, `quality-engineer`, `devops-engineer`) must produce findings-first output with this section order:
  1. Findings
  2. Evidence
  3. Risk
  4. Required Fix
  5. Verification
- Severity enum for findings: `critical`, `high`, `medium`, `low`.
- Findings must be ordered highest severity first and include file/line references when applicable.

- Implementation role (`software-engineer`) must produce this section order:
  1. Scope
  2. Change Plan
  3. Touched Files
  4. Validation

### Recommended command baseline
- Core checks:
  - `go test ./...`
  - `go vet ./...`
  - `go build ./...`
- Extended checks (if tooling exists):
  - `gosec ./...`
  - image/container scan (for example Trivy)
  - Helm validation (`helm lint`, `helm template`)
- If an extended tool is unavailable, report `not-run` with reason. Do not fabricate results.
