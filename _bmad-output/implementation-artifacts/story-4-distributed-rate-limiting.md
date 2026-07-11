# Story 1.4: Validated Rate Limiting for Multi-Replica Deployments

Status: completed

## Story

As a security engineer,
I want auth failure rate limiting to work correctly regardless of replica count,
so that brute-force attacks are reliably throttled in multi-replica production deployments.

## Acceptance Criteria

1. An ADR documents the chosen approach: ingress-layer (Traefik) vs. Redis-backed vs. single-replica-only with explicit documentation
2. If ingress-layer: Helm chart includes Traefik `IngressRoute` with `RateLimit` middleware; example values provided
3. If Redis: new `RATE_LIMIT_REDIS_URL` env var; Redis connection failure degrades gracefully to in-memory with a logged warning
4. Integration test: 65+ failed auth requests within 60s → subsequent requests receive `429 Too Many Requests`
5. Integration test: legitimate requests from a different IP are NOT rate-limited during the above scenario
6. `values.yaml` and README explicitly document behaviour when `replicaCount > 1` with in-memory rate limiting

## Tasks / Subtasks

- [ ] Task 1: Architectural decision (AC: 1)
  - [ ] Evaluate ingress-layer (Traefik middleware) vs. Redis vs. document-the-limitation approaches
  - [ ] Write ADR to `docs/adr/0001-rate-limiting-strategy.md`
  - [ ] Get decision confirmed before proceeding to implementation

- [ ] Task 2: Implement chosen approach (AC: 2 or 3)
  - [ ] **If Traefik:** Add `helm/clock-server/templates/ingressroute.yaml` with RateLimit middleware; add `rateLimit` block to `values.yaml`
  - [ ] **If Redis:** Add `RATE_LIMIT_REDIS_URL` to `internal/config/config.go`; implement Redis sliding window in `internal/api/ratelimit.go`; fallback to in-memory on connection failure with `log.Warn`
  - [ ] **If document-only:** Add explicit warning to `values.yaml` and chart README: "in-memory rate limiting is per-replica; set `replicaCount: 1` or use ingress-layer rate limiting for multi-replica"

- [ ] Task 3: Integration tests (AC: 4, 5)
  - [ ] Add integration test (Docker Compose or Go `httptest`) that fires 65 bad-token requests within 60s from IP A → assert 429 returned
  - [ ] Assert requests from IP B (different source) are NOT blocked during the above
  - [ ] Test that after the 60s window resets, requests from IP A are accepted again (window expiry)

- [ ] Task 4: Documentation (AC: 6)
  - [ ] Add comment block in `helm/clock-server/values.yaml` under `replicaCount` explaining rate limiting behaviour
  - [ ] Update chart README with multi-replica rate limiting guidance

## Dev Notes

- Current rate limiter is in `internal/api/handlers.go` — the `authFailureLimiter` struct with sliding window map
- The `RateLimiter` interface (if one exists) should make swapping backends clean; if not, extract one before adding Redis
- For Redis: use `github.com/redis/go-redis/v9` — minimal, well-maintained
- Redis key scheme: `clock-server:ratelimit:{ip}:{token_hash_prefix}` with TTL matching the window (60s)
- Graceful degradation: if Redis `Ping()` fails at startup, log warning and use in-memory; don't crash
- Traefik approach is lower complexity and recommended if Traefik is already in the stack
- Race detector must pass — rate limiter map accesses must be mutex-protected (already the case for in-memory impl)

### Project Structure Notes

- New: `docs/adr/0001-rate-limiting-strategy.md`
- Modified (if Redis): `internal/api/handlers.go` or new `internal/api/ratelimit.go`
- Modified (if Redis): `internal/config/config.go`
- Modified (if Traefik): `helm/clock-server/templates/`, `helm/clock-server/values.yaml`
- New tests: integration test file in `internal/api/` or `test/integration/`

### References

- [Source: internal/api/handlers.go] — Current `authFailureLimiter` implementation
- [Source: internal/config/config.go] — `AUTH_FAIL_LIMIT_PER_MIN` config pattern
- [Source: docker-compose.yml] — Integration test environment reference

## Dev Agent Record

### Agent Model Used

claude-sonnet-4-6

### Debug Log References

### Completion Notes List

### File List

- `docs/adr/0001-rate-limiting-strategy.md` — ADR documenting the three options and decision
- `helm/clock-server/templates/traefik-ratelimit.yaml` — Traefik Middleware CRD template
- `helm/clock-server/templates/ingress.yaml` — updated to apply middleware annotation when enabled
- `helm/clock-server/values.yaml` — added `rateLimit` block and replicaCount documentation comment
- `internal/api/ratelimit_integration_test.go` — integration tests for in-memory rate limiter
