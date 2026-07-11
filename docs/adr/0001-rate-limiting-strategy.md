# ADR 0001: Rate Limiting Strategy for Multi-Replica Deployments

**Status:** Accepted
**Date:** 2026-03-13
**Deciders:** Engineering team

---

## Context

The clock-server implements an in-memory sliding-window rate limiter (`authFailureLimiter`) that blocks
IP/token combinations after too many authentication failures within a 60-second window. This protects
against brute-force credential attacks.

When `replicaCount > 1`, each pod maintains its own independent rate-limiter state. An attacker can
cycle across replicas — sending up to `authFailLimitPerMin` failures per minute to each pod — and
only be blocked per-replica rather than cluster-wide. This weakens the protection in horizontally
scaled deployments.

Three options were evaluated to address this:

---

## Options Considered

### Option 1: In-Memory Rate Limiter (Current State)

**Description:** Each replica maintains its own sliding-window map keyed by `(IP, token-hash)`.

**Pros:**
- Zero dependencies — no external services required
- Sub-microsecond latency for rate-limit checks
- Already implemented and tested
- Stateless pods — easy to scale and restart

**Cons:**
- Per-replica, not cluster-wide: an attacker can target different replicas to bypass the limit
- State is lost on pod restart
- Not suitable as the sole mechanism for `replicaCount > 1`

**Verdict:** Insufficient alone for multi-replica deployments. Retained as defence-in-depth.

---

### Option 2: Redis-Backed Distributed Rate Limiter

**Description:** Replace (or augment) the in-memory store with a Redis sorted set implementing a
sliding-window counter. Key scheme: `clock-server:ratelimit:{ip}:{token_hash_prefix}` with TTL
matching the window. Graceful degradation: if Redis is unavailable, fall back to in-memory with a
logged warning.

**Pros:**
- True cluster-wide rate limiting across all replicas
- State survives pod restarts
- Well-understood pattern; `github.com/redis/go-redis/v9` is minimal and well-maintained

**Cons:**
- Introduces a new runtime dependency (Redis deployment, HA configuration, network latency)
- Adds operational complexity: Redis must be monitored, backed up, and sized
- Graceful degradation means the protection is silently weakened during Redis outages
- Adds latency on every authenticated request path (~1 ms per round-trip)
- Not required by any other part of the system today

**Verdict:** Technically sound but operationally expensive for the current scale. Deferred.

---

### Option 3: Ingress-Layer Rate Limiting via Traefik Middleware (Selected)

**Description:** Configure a Traefik `Middleware` resource (kind: `Middleware`,
apiVersion: `traefik.io/v1alpha1`) with `rateLimit` settings. Reference it from the `Ingress`
resource via the `traefik.ingress.kubernetes.io/router.middlewares` annotation.

**Pros:**
- Traefik is already in the stack — no new runtime dependency
- Rate limiting happens at the ingress layer, before requests reach the application pods
- Scales horizontally naturally: Traefik (or its backing store) is the single enforcement point
- Configuration is declarative (Helm values); no application code changes required
- Keeps the application stateless

**Cons:**
- Rate limiting only applies to traffic routed through the ingress. Direct pod access (e.g.,
  `kubectl port-forward`, intra-cluster service calls) bypasses it.
- Traefik's built-in rate limiter uses an in-memory store per Traefik instance; for Traefik HA
  deployments, a Redis-backed Traefik plugin or sticky routing should be considered.
- Less granular than application-level control (operates on IP, not IP+token-hash)

**Verdict:** Best fit for current operational requirements. Lowest complexity, no new dependencies.

---

## Decision

**Ingress-layer rate limiting via Traefik Middleware** (Option 3).

A Traefik `Middleware` resource is added to the Helm chart, guarded by
`rateLimit.traefik.enabled` (default `false`). When enabled, the `Ingress` resource is annotated
to apply the middleware. Default values: 60 requests/minute average, burst of 10.

---

## Rationale

- Traefik is already the ingress controller in this stack; adding a `Middleware` CRD is a
  configuration change, not an infrastructure change.
- The application already enforces per-replica rate limiting as defence-in-depth. The ingress layer
  adds cluster-wide enforcement without modifying application code.
- Keeps the application stateless and operationally simple.
- Redis can be added later (Option 2) if Traefik HA or intra-cluster rate limiting becomes a
  requirement.

---

## Consequences

### Positive
- Cluster-wide rate limiting for external traffic with zero new runtime dependencies.
- Declarative, version-controlled configuration via Helm values.
- Application remains stateless.

### Negative / Trade-offs
- **Direct pod access bypasses ingress-layer rate limiting.** The in-memory limiter in the
  application remains active as defence-in-depth for this case.
- **Traefik HA caveat:** If Traefik itself runs as multiple replicas without a shared backing store,
  each Traefik instance enforces the limit independently. For HA Traefik, evaluate the Traefik Redis
  plugin or use sticky sessions.
- **Granularity:** Traefik's rate limiter operates on source IP, not IP+token-hash. This is less
  surgical than the application-layer limiter but sufficient for ingress-level protection.

### Retained Behaviour
- The in-memory `authFailureLimiter` in the application is **not removed**. It continues to protect
  against auth brute-force from internal/direct traffic and acts as a second layer of defence for
  ingress traffic.
- `values.yaml` documents the per-replica behaviour and instructs operators to enable
  `rateLimit.traefik.enabled=true` for cluster-wide protection.
