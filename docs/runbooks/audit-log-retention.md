# Runbook: Audit Log Retention

## Overview

clock-server emits one structured JSON line to **stderr** for every command
request it processes.  Every line conforms to the `AuditEntry` schema defined
in `internal/api/audit.go`.

Required fields on every entry:

| Field        | Type   | Description                                    |
|--------------|--------|------------------------------------------------|
| `principal`  | string | Credential ID of the authenticated caller      |
| `remote`     | string | Client IP address                              |
| `method`     | string | HTTP method (POST, PUT, …)                     |
| `path`       | string | Request URL path                               |
| `device`     | string | Target clock device ID                         |
| `command`    | string | Command type (set_alarm, display_message, …)   |
| `result`     | string | `accepted` or `failed`                         |
| `request_id` | string | X-Request-Id echo (auto-generated if absent)   |
| `timestamp`  | string | RFC 3339 UTC timestamp                         |

Sensitive values (`Authorization` header content, raw tokens, passwords) are
**never** written to the audit log.

---

## Recommended Retention Policy

| Tier         | Duration    | Rationale                                              |
|--------------|-------------|--------------------------------------------------------|
| Hot storage  | 30 days     | Fast indexed search for ongoing incident response      |
| Warm storage | 60 days     | Compliance queries, slower but still searchable        |
| Cold archive | 90 days min | Regulatory minimum; extend to 1 year if required       |

> **Minimum recommended retention: 90 days.**
> Many compliance frameworks (SOC 2, ISO 27001) require at least 90 days of
> accessible audit records.  Consult your organisation's compliance team before
> reducing this threshold.

---

## Log Shipping

### Option A — Fluent Bit Sidecar (Kubernetes)

Enable the sidecar in `helm/clock-server/values.yaml`:

```yaml
logging:
  fluentbit:
    enabled: true
    image: fluent/fluent-bit:3.3
```

Add a minimal Fluent Bit configuration ConfigMap that parses JSON and forwards
to your backend.  Example for Loki:

```ini
[INPUT]
    Name        tail
    Path        /var/log/clock-server/*.log
    Parser      json
    Tag         clock-server

[OUTPUT]
    Name        loki
    Match       clock-server
    Host        loki.monitoring.svc.cluster.local
    Port        3100
    Labels      app=clock-server
```

### Option B — Fluent Bit DaemonSet (cluster-wide)

Add the pod annotation so the cluster-level collector picks up this pod's logs
and applies the JSON parser:

```yaml
podAnnotations:
  fluentbit.io/parser: json
```

### Option C — Elasticsearch / OpenSearch

Configure a Logstash or Vector pipeline to ingest `stderr` from the pod and
index each JSON document.  Recommended index settings:

```json
{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 1
  },
  "mappings": {
    "properties": {
      "timestamp":  { "type": "date" },
      "principal":  { "type": "keyword" },
      "remote":     { "type": "ip" },
      "method":     { "type": "keyword" },
      "path":       { "type": "keyword" },
      "device":     { "type": "keyword" },
      "command":    { "type": "keyword" },
      "result":     { "type": "keyword" },
      "request_id": { "type": "keyword" }
    }
  }
}
```

Use an ILM (Index Lifecycle Management) policy to roll over the index after
30 days and delete after 90 days (or archive to cheaper storage first).

---

## Querying Audit Logs

### Find all events for a specific `request_id`

**Loki (LogQL):**

```logql
{app="clock-server"} | json | request_id="<YOUR-REQUEST-ID>"
```

**Elasticsearch (Query DSL):**

```json
{
  "query": {
    "term": { "request_id": "<YOUR-REQUEST-ID>" }
  }
}
```

**jq (raw log files):**

```bash
grep '"request_id"' /var/log/clock-server/audit.log \
  | jq 'select(.request_id == "<YOUR-REQUEST-ID>")'
```

### Find all failed commands by a principal

**Loki:**

```logql
{app="clock-server"} | json | principal="<PRINCIPAL-ID>" | result="failed"
```

**jq:**

```bash
jq 'select(.principal == "<PRINCIPAL-ID>" and .result == "failed")' \
  /var/log/clock-server/audit.log
```

### Count failed commands in the last hour

**Loki:**

```logql
count_over_time(
  {app="clock-server"} | json | result="failed" [1h]
)
```

---

## Alerting Recommendations

| Alert                               | Condition                                 | Severity |
|-------------------------------------|-------------------------------------------|----------|
| High auth failure rate              | >10 `result=failed` per minute, same IP   | Warning  |
| Unknown principal                   | `principal="unknown"` in audit log        | Warning  |
| Audit log gap                       | No audit lines for >5 minutes during peak | Critical |
| Sensitive keyword in log            | `Authorization`, `token`, or `password`   | Critical |

---

## References

- `internal/api/audit.go` — AuditEntry struct and writeAuditEntry function
- `internal/api/audit_test.go` — Schema regression and negative (sensitive-data) tests
- `helm/clock-server/values.yaml` — Fluent Bit sidecar configuration
