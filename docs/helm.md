← [Back to README](../README.md)

# Helm Chart — `helm/clock-server`

| Field | Value |
|-------|-------|
| Chart name | `clock-server` |
| Chart version | `0.1.0` |
| App version | `0.1.0` |
| Type | `application` |

The chart deploys the clock command dispatcher service as a Kubernetes Deployment with a Service, optional Ingress, a ServiceAccount, and an authentication Secret.

## Prerequisites

- Helm 3.x
- Kubernetes 1.19+ (uses `networking.k8s.io/v1` Ingress)
- A container image of clock-server pushed to a registry accessible by the cluster

## Quick Install

Create a minimal values override file:

```yaml
# my-values.yaml
image:
  repository: registry.example.com/clock-server
  tag: "0.1.0"

auth:
  credentials: "admin|s3cret|*"

config:
  enabledSenders: "mqtt"
  mqtt:
    brokerURL: "mqtt://broker:1883"
```

Install (or upgrade) the release:

```bash
helm upgrade --install clock-server ./helm/clock-server \
  -n clock-server --create-namespace \
  -f my-values.yaml
```

## Values Reference

### Root

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `replicaCount` | int | `1` | Number of pod replicas |
| `nameOverride` | string | `""` | Override the chart name used in resource names |
| `fullnameOverride` | string | `""` | Fully override the generated resource name |
| `podAnnotations` | object | `{}` | Extra annotations added to the pod template |
| `podLabels` | object | `{}` | Extra labels added to the pod template |
| `nodeSelector` | object | `{}` | Node selector constraints |
| `tolerations` | list | `[]` | Tolerations for pod scheduling |
| `affinity` | object | `{}` | Affinity rules for pod scheduling |

### Image

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `image.repository` | string | `clock-server` | Container image repository |
| `image.tag` | string | `local` | Image tag |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `imagePullSecrets` | list | `[]` | Registry pull secret references |

### Service Account

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `serviceAccount.create` | bool | `true` | Create a dedicated ServiceAccount |
| `serviceAccount.automount` | bool | `false` | Automount the service account token into pods |
| `serviceAccount.annotations` | object | `{}` | Annotations on the ServiceAccount |
| `serviceAccount.name` | string | `""` | Override the ServiceAccount name (defaults to fullname) |

### Pod Security Context

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `podSecurityContext.runAsNonRoot` | bool | `true` | Require the container to run as a non-root user |
| `podSecurityContext.seccompProfile.type` | string | `RuntimeDefault` | Seccomp profile type |

### Container Security Context

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `securityContext.allowPrivilegeEscalation` | bool | `false` | Disallow privilege escalation |
| `securityContext.readOnlyRootFilesystem` | bool | `true` | Mount root filesystem as read-only |
| `securityContext.capabilities.drop` | list | `["ALL"]` | Linux capabilities to drop |

### Service

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `service.type` | string | `ClusterIP` | Kubernetes Service type |
| `service.port` | int | `8080` | Service port |

### Ingress

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ingress.enabled` | bool | `false` | Enable Ingress resource creation |
| `ingress.className` | string | `"traefik"` | Ingress class name |
| `ingress.annotations` | object | `{}` | Ingress annotations |
| `ingress.hosts` | list | see below | List of host rules |
| `ingress.hosts[].host` | string | `clock-server.local` | Hostname |
| `ingress.hosts[].paths[].path` | string | `/` | URL path |
| `ingress.hosts[].paths[].pathType` | string | `Prefix` | Path match type |
| `ingress.tls` | list | `[]` | TLS configuration |

### Resources

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `resources.requests.cpu` | string | `50m` | CPU request |
| `resources.requests.memory` | string | `64Mi` | Memory request |
| `resources.limits.cpu` | string | `250m` | CPU limit |
| `resources.limits.memory` | string | `256Mi` | Memory limit |

### Probes

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `probes.liveness.path` | string | `/health` | Liveness probe HTTP path |
| `probes.liveness.initialDelaySeconds` | int | `5` | Seconds before first liveness probe |
| `probes.liveness.periodSeconds` | int | `10` | Interval between liveness probes |
| `probes.liveness.timeoutSeconds` | int | `2` | Liveness probe timeout |
| `probes.liveness.failureThreshold` | int | `3` | Failures before pod is restarted |
| `probes.readiness.path` | string | `/health` | Readiness probe HTTP path |
| `probes.readiness.initialDelaySeconds` | int | `3` | Seconds before first readiness probe |
| `probes.readiness.periodSeconds` | int | `10` | Interval between readiness probes |
| `probes.readiness.timeoutSeconds` | int | `2` | Readiness probe timeout |
| `probes.readiness.failureThreshold` | int | `3` | Failures before pod is marked unready |

### Auth

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `auth.existingSecret` | string | `""` | Name of a pre-existing Secret to use instead of creating one |
| `auth.credentials` | string | `""` | Pipe-delimited credentials string (`user\|pass\|scope`) |
| `auth.legacyToken` | string | `""` | Legacy bearer token for API authentication |
| `auth.secretKeys.credentials` | string | `API_AUTH_CREDENTIALS` | Key inside the Secret that holds the credentials value |
| `auth.secretKeys.token` | string | `API_AUTH_TOKEN` | Key inside the Secret that holds the legacy token value |

### Config — General

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `config.httpAddr` | string | `":8080"` | Listen address |
| `config.requireTLS` | bool | `false` | Reject requests that did not arrive over TLS |
| `config.trustProxyTLS` | bool | `true` | Trust `X-Forwarded-Proto` header from a reverse proxy |
| `config.readinessRequireAuth` | bool | `false` | Require authentication on the readiness endpoint |
| `config.maxBodyBytes` | int | `65536` | Maximum request body size in bytes |
| `config.authFailLimitPerMin` | int | `60` | Rate limit for authentication failures per minute |
| `config.httpReadTimeoutMs` | int | `10000` | HTTP read timeout (ms) |
| `config.httpWriteTimeoutMs` | int | `10000` | HTTP write timeout (ms) |
| `config.httpIdleTimeoutMs` | int | `60000` | HTTP idle timeout (ms) |
| `config.httpReadHeaderTimeoutMs` | int | `5000` | HTTP read-header timeout (ms) |
| `config.httpShutdownTimeoutMs` | int | `15000` | Graceful shutdown timeout (ms) |
| `config.enabledSenders` | string | `""` | Comma-separated list of enabled senders (`mqtt`, `rest`) |

### Config — MQTT Sender

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `config.mqtt.brokerURL` | string | `""` | MQTT broker URL (required when `mqtt` sender is enabled) |
| `config.mqtt.clientID` | string | `""` | MQTT client identifier |
| `config.mqtt.username` | string | `""` | MQTT username |
| `config.mqtt.password` | string | `""` | MQTT password |
| `config.mqtt.topicPrefix` | string | `"clocks/commands"` | MQTT topic prefix |
| `config.mqtt.qos` | int | `1` | MQTT QoS level (0, 1, or 2) |
| `config.mqtt.retained` | bool | `false` | Publish messages with the retained flag |
| `config.mqtt.connectRetry` | bool | `true` | Retry connecting to the broker on failure |
| `config.mqtt.tlsInsecureSkipVerify` | bool | `false` | Skip TLS certificate verification for the broker |
| `config.mqtt.allowInsecureTLSVerify` | bool | `false` | Allow insecure TLS verification (must be explicitly enabled) |
| `config.mqtt.allowInsecureMQTT` | bool | `false` | Allow plain `mqtt://` connections (non-TLS) |

### Config — REST Sender

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `config.rest.baseURL` | string | `""` | Downstream REST API base URL (required when `rest` sender is enabled) |
| `config.rest.token` | string | `""` | Bearer token for the downstream API |
| `config.rest.timeoutMs` | int | `5000` | Request timeout (ms) |
| `config.rest.healthPath` | string | `""` | Health-check path on the downstream API |
| `config.rest.allowInsecureHTTP` | bool | `false` | Allow plain HTTP connections to the downstream API |

## Templates

| Template | Resource | Description |
|----------|----------|-------------|
| `deployment.yaml` | `Deployment` | Runs the clock-server container. Injects all `config.*` values as environment variables, mounts auth credentials from a Secret, configures liveness/readiness probes on `/health`, and mounts an emptyDir at `/tmp` (required because the root filesystem is read-only). |
| `service.yaml` | `Service` | Exposes the Deployment on the configured `service.port` (default 8080) targeting the `http` named port. |
| `serviceaccount.yaml` | `ServiceAccount` | Created when `serviceAccount.create` is `true`. Token automount is disabled by default. |
| `secret-auth.yaml` | `Secret` (Opaque) | Created only when `auth.existingSecret` is empty **and** at least one of `auth.credentials` or `auth.legacyToken` is set. Stores both values under the keys defined in `auth.secretKeys`. |
| `ingress.yaml` | `Ingress` | Created when `ingress.enabled` is `true`. Supports multiple hosts/paths and optional TLS. |
| `validate.yaml` | — (fail templates) | Helm install-time validation rules (see [Chart Validation](#chart-validation--constraints) below). |
| `NOTES.txt` | — (post-install notes) | Prints kubectl commands to verify the deployment and a sample curl command. |
| `_helpers.tpl` | — (template helpers) | Defines `clock-server.name`, `clock-server.fullname`, `clock-server.chart`, `clock-server.labels`, `clock-server.selectorLabels`, `clock-server.serviceAccountName`, and `clock-server.authSecretName`. |

## Security Posture

The chart ships with a hardened-by-default security configuration:

- **Pod security context**: `runAsNonRoot: true` with `seccompProfile: RuntimeDefault`.
- **Container security context**: `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, all Linux capabilities dropped (`DROP ALL`).
- **Writable volume**: An emptyDir is mounted at `/tmp` to provide the only writable path.
- **Service account**: Created per-release with `automountServiceAccountToken: false` to avoid exposing the Kubernetes API token.
- **Auth secret references**: Mounted via `secretKeyRef` with `optional: true`, so the pod still starts if keys are missing (validation catches this at install time instead).
- **Rate limiting**: `authFailLimitPerMin` (default 60) limits brute-force attempts.
- **TLS controls**: `requireTLS`, `trustProxyTLS`, and per-sender `allowInsecure*` flags give fine-grained control over transport security.

## Ingress

Ingress is disabled by default. To enable it with Traefik (the default `className`):

```yaml
ingress:
  enabled: true
  className: "traefik"
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
  hosts:
    - host: clock.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: clock-tls
      hosts:
        - clock.example.com
```

To use a different ingress controller, change `className` (e.g., `"nginx"`) and adjust annotations accordingly.

When ingress is enabled, the post-install notes print the ingress URL(s). When disabled, they print a `kubectl port-forward` command instead.

## Authentication Secrets

The chart supports two patterns for providing API authentication credentials:

### Inline credentials (simple)

Set `auth.credentials` and/or `auth.legacyToken` directly in your values file. The chart creates a Secret named `<fullname>-auth`:

```yaml
auth:
  credentials: "admin|s3cret|*"
  legacyToken: "my-legacy-token"
```

### External Secret (recommended for production)

Point to a pre-existing Secret and skip inline values entirely:

```yaml
auth:
  existingSecret: "my-precreated-secret"
```

The Secret must contain the keys defined by `auth.secretKeys.credentials` (`API_AUTH_CREDENTIALS`) and `auth.secretKeys.token` (`API_AUTH_TOKEN`). You can override these key names:

```yaml
auth:
  existingSecret: "my-secret"
  secretKeys:
    credentials: MY_CREDS_KEY
    token: MY_TOKEN_KEY
```

When `existingSecret` is set, `auth.credentials` and `auth.legacyToken` are ignored, and no chart-managed Secret is created.

## Chart Validation / Constraints

The `validate.yaml` template uses Helm `fail` to enforce the following rules at install/upgrade time:

1. **Auth is required** — at least one of `auth.credentials`, `auth.legacyToken`, or `auth.existingSecret` must be set. Failure message:
   > *Set auth.credentials/auth.legacyToken or auth.existingSecret (API auth is required).*

2. **MQTT broker URL required** — if `config.enabledSenders` includes `mqtt`, then `config.mqtt.brokerURL` must be non-empty. Failure message:
   > *config.mqtt.brokerURL is required when config.enabledSenders includes mqtt.*

3. **REST base URL required** — if `config.enabledSenders` includes `rest`, then `config.rest.baseURL` must be non-empty. Failure message:
   > *config.rest.baseURL is required when config.enabledSenders includes rest.*

The `_helpers.tpl` file provides standard naming helpers (`name`, `fullname`, `chart`, `labels`, `selectorLabels`, `serviceAccountName`, `authSecretName`) that truncate names to 63 characters to comply with Kubernetes naming limits.

## Uninstall

```bash
helm uninstall clock-server -n clock-server
```

To also remove the namespace:

```bash
kubectl delete namespace clock-server
```

Note: if you used `auth.existingSecret`, the external Secret is **not** deleted by `helm uninstall` since it is not managed by the chart.
