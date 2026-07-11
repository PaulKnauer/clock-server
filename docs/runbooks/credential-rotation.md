# Runbook: Zero-Downtime Credential Rotation

## Overview

The clock-server supports hot credential rotation via `SIGHUP`. Sending `SIGHUP` to the
server process causes it to re-read `API_AUTH_CREDENTIALS` from the environment and atomically
swap the in-memory credential store — without dropping any in-flight requests and without a
process restart.

## Recommended Rotation Frequency

Rotate API credentials at least every **90 days**. Rotate immediately if a credential is
suspected of being compromised.

---

## Procedure: SIGHUP-based Hot Reload (preferred)

This approach achieves zero downtime. There is no window where all credentials are invalid;
the old set remains active until the new set is successfully loaded.

### Step 1 — Update the Kubernetes Secret

```bash
# Encode the new credential string (no newline)
NEW_CREDS="ops|<new-token>|*"
kubectl create secret generic clock-server-credentials \
  --from-literal=API_AUTH_CREDENTIALS="$NEW_CREDS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Or patch an existing secret directly:

```bash
kubectl patch secret clock-server-credentials \
  --type='json' \
  -p="[{\"op\":\"replace\",\"path\":\"/data/API_AUTH_CREDENTIALS\",\"value\":\"$(echo -n "$NEW_CREDS" | base64)\"}]"
```

### Step 2 — Trigger credential reload via SIGHUP

The Kubernetes Secret update does **not** automatically propagate to running pods. You must
explicitly signal the process.

```bash
# Replace <pod-name> with the actual pod name (use kubectl get pods to find it).
kubectl exec <pod-name> -- kill -HUP 1
```

`kill -HUP 1` sends `SIGHUP` to PID 1 inside the container (the clock-server process).

For all pods in the deployment at once:

```bash
for pod in $(kubectl get pods -l app=clock-server -o name); do
  kubectl exec $pod -- kill -HUP 1
done
```

### Step 3 — Verify the reload

Check that the server logged a successful reload (no token values are logged):

```bash
kubectl logs <pod-name> | grep "credentials reloaded"
# Expected output:
# credentials reloaded, count=1 ids=[ops]
```

Verify the old token is rejected and the new token is accepted:

```bash
# Old token — must return 401
curl -sf -H "Authorization: Bearer <old-token>" https://<host>/ready || echo "correctly rejected"

# New token — must return 200
curl -sf -H "Authorization: Bearer <new-token>" https://<host>/ready && echo "correctly accepted"
```

---

## Alternative: Rolling Restart

If `kubectl exec` access is unavailable, or you prefer a Kubernetes-native approach, trigger
a rolling restart. This replaces pods one-by-one using the new Secret, ensuring availability
throughout the rollout.

```bash
# 1. Update the Secret (same as Step 1 above).

# 2. Restart the deployment.
kubectl rollout restart deployment/clock-server

# 3. Wait for the rollout to complete.
kubectl rollout status deployment/clock-server
```

**Trade-offs:**
- Simpler: no `kubectl exec` required.
- Slightly slower: pods are recycled rather than reloaded in-place.
- Brief dual-version window: old and new pods coexist during the rollout. Both the old
  credential and the new credential must be accepted during this window if overlap is
  required. To support overlap, include both credentials in `API_AUTH_CREDENTIALS` during
  the transition:
  `old-ops|<old-token>|*;ops|<new-token>|*`
  Then remove the old entry in a subsequent rotation.

---

## Failure Modes

| Scenario | Behaviour |
|---|---|
| `API_AUTH_CREDENTIALS` is malformed at reload time | Error is logged; existing credentials remain active (fail-safe). |
| `API_AUTH_CREDENTIALS` is empty at reload time | Reload is skipped; existing credentials remain active. |
| Pod restarts after Secret update | New pod reads the updated Secret on startup; no manual reload needed. |
| Secret update races with SIGHUP | The value read at SIGHUP time is used. If the Secret has not propagated yet, the old value is re-parsed (idempotent). |

---

## Security Notes

- Token values are **never** written to logs. Only credential IDs and counts are logged.
- Keep the rotation window short: update the Secret and send SIGHUP in the same maintenance
  window.
- After confirming the new credential works, revoke the old credential from any downstream
  systems that may have it cached.
