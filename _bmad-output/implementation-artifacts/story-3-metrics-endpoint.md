# Story 1.3: Add Prometheus Metrics Endpoint

Status: completed

## Story

As an SRE,
I want a `/metrics` endpoint exposing Prometheus-format counters and histograms,
so that I can monitor service health and alert on anomalous behaviour in production.

## Acceptance Criteria

1. `GET /metrics` returns valid Prometheus text format exposition
2. Metrics exposed: `clock_requests_total{method,path,status}`, `clock_auth_failures_total{reason}`, `clock_dispatch_duration_seconds{adapter}`, `clock_dispatch_errors_total{adapter}`
3. `METRICS_REQUIRE_AUTH` env var (default `false`) controls whether `/metrics` requires bearer auth
4. Helm chart enables optional Prometheus `ServiceMonitor` resource (off by default via `metrics.serviceMonitor.enabled`)
5. Unit tests assert metric counters increment correctly on success and failure paths
6. Example Prometheus alert rule provided for auth failure rate spike: fire if rate > 10/min over 5 minutes
7. `/metrics` is excluded from auth failure rate limiting
8. No new external dependencies introduced — use Go stdlib or the standard `prometheus/client_golang` library

## Tasks / Subtasks

- [ ] Task 1: Add `prometheus/client_golang` dependency (AC: 8)
  - [ ] `go get github.com/prometheus/client_golang`
  - [ ] Confirm this is the only new production dependency added

- [ ] Task 2: Define and register metrics (AC: 2)
  - [ ] Create `internal/metrics/metrics.go` with metric definitions
  - [ ] `clock_requests_total` — CounterVec with labels `method`, `path`, `status`
  - [ ] `clock_auth_failures_total` — CounterVec with label `reason` (values: `missing_token`, `invalid_token`, `invalid_scope`, `rate_limited`)
  - [ ] `clock_dispatch_duration_seconds` — HistogramVec with label `adapter`
  - [ ] `clock_dispatch_errors_total` — CounterVec with label `adapter`

- [ ] Task 3: Instrument handlers and dispatcher (AC: 2)
  - [ ] Increment `clock_requests_total` in `internal/api/handlers.go` after each response
  - [ ] Increment `clock_auth_failures_total` at each auth failure point
  - [ ] Record `clock_dispatch_duration_seconds` and `clock_dispatch_errors_total` in `internal/application/dispatcher.go`

- [ ] Task 4: Add `/metrics` HTTP endpoint (AC: 1, 3, 7)
  - [ ] Register `promhttp.Handler()` at `/metrics` in handler setup
  - [ ] If `METRICS_REQUIRE_AUTH=true`, wrap with bearer auth middleware (reuse existing auth logic)
  - [ ] Exclude `/metrics` path from auth failure rate limiting logic in `internal/api/handlers.go`
  - [ ] Add `METRICS_REQUIRE_AUTH` to `internal/config/config.go`

- [ ] Task 5: Helm ServiceMonitor (AC: 4)
  - [ ] Add `helm/clock-server/templates/servicemonitor.yaml` guarded by `metrics.serviceMonitor.enabled`
  - [ ] Add `metrics` block to `helm/clock-server/values.yaml`

- [ ] Task 6: Tests (AC: 5)
  - [ ] Unit test: successful request → `clock_requests_total` incremented with correct labels
  - [ ] Unit test: auth failure → `clock_auth_failures_total` incremented with correct reason label
  - [ ] Unit test: dispatcher error → `clock_dispatch_errors_total` incremented
  - [ ] Unit test: `/metrics` returns 200 with `text/plain` content type

- [ ] Task 7: Alert rule documentation (AC: 6)
  - [ ] Add `docs/alerts/auth-failure-spike.yaml` with example PrometheusRule
  - [ ] Document alert rationale and recommended threshold tuning

## Dev Notes

- Keep metrics in a dedicated `internal/metrics/` package to avoid circular imports
- Use `prometheus.MustRegister` at package init or via explicit registry — prefer explicit registry to avoid global state in tests
- Pass metrics registry into handlers via dependency injection (consistent with existing hexagonal pattern)
- The `promhttp.Handler()` serves the default registry — if using a custom registry, use `promhttp.HandlerFor(registry, promhttp.HandlerOpts{})`
- For histograms, use default buckets unless there's a known latency SLO to target
- Race detector: metric increments must be goroutine-safe — `prometheus/client_golang` handles this internally

### Project Structure Notes

- New package: `internal/metrics/metrics.go`
- Modified: `internal/api/handlers.go` — instrument + add `/metrics` route
- Modified: `internal/application/dispatcher.go` — dispatch duration/error metrics
- Modified: `internal/config/config.go` — `METRICS_REQUIRE_AUTH`
- New: `helm/clock-server/templates/servicemonitor.yaml`
- Modified: `helm/clock-server/values.yaml`
- New: `docs/alerts/auth-failure-spike.yaml`

### References

- [Source: internal/api/handlers.go] — Handler structure and rate limiter exclusion pattern
- [Source: internal/application/dispatcher.go] — Dispatch flow to instrument
- [Source: internal/config/config.go] — Config loading pattern
- [Source: helm/clock-server/templates/] — Existing template patterns

## Dev Agent Record

### Agent Model Used

claude-sonnet-4-6

### Debug Log References

### Completion Notes List

### File List

**New files:**
- `internal/metrics/metrics.go` — Metrics struct with explicit prometheus.NewRegistry(), all four metric definitions
- `internal/metrics/metrics_test.go` — Unit tests for metrics registration and isolation
- `internal/api/metrics_test.go` — Unit tests for handler instrumentation (requests_total, auth_failures_total, /metrics endpoint behaviour)
- `helm/clock-server/templates/servicemonitor.yaml` — ServiceMonitor resource guarded by `metrics.serviceMonitor.enabled`
- `docs/alerts/auth-failure-spike.yaml` — Example PrometheusRule for auth failure spike alert

**Modified files:**
- `internal/api/handlers.go` — Added HandlerOptions/NewHandlerWithOptions, metrics field, metricsMiddleware, statusRecorder, /metrics route, auth failure metric instrumentation, /metrics rate-limiter exclusion
- `internal/config/config.go` — Added MetricsRequireAuth field and METRICS_REQUIRE_AUTH env var (default false)
- `cmd/server/main.go` — Wired metrics.NewMetrics() and HandlerOptions into handler construction
- `helm/clock-server/values.yaml` — Added metrics block (serviceMonitor config) and config.metricsRequireAuth
- `helm/clock-server/templates/deployment.yaml` — Added METRICS_REQUIRE_AUTH env var
- `go.mod` / `go.sum` — Added github.com/prometheus/client_golang and transitive dependencies
