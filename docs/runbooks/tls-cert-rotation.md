# TLS Certificate Rotation Runbook

## Overview

This runbook covers TLS certificate lifecycle management for clock-server, including
automatic renewal via cert-manager and manual rotation procedures for environments
without cert-manager.

---

## Alert Threshold

clock-server logs a **WARNING** at startup when the loaded TLS certificate expires within
**30 days**. This is a non-fatal warning; the server will still start and serve traffic.

Example log line:

```
WARNING: TLS certificate expires in 14 days (2026-04-01T00:00:00Z); renew soon
```

If the certificate is **already expired** at startup, clock-server exits with a non-zero
status and logs an error:

```
TLS certificate check failed: TLS certificate expired on 2026-03-01T00:00:00Z
```

**Recommended action when the 30-day warning fires:** initiate certificate renewal
immediately using one of the procedures below.

---

## Automatic Renewal via cert-manager

### How it works

When `tls.certManager.enabled: true` in `values.yaml`, the Helm chart creates a
cert-manager `Certificate` resource. cert-manager monitors the certificate and
automatically renews it `renewBefore` (default: 720h / 30 days) before expiry.

The renewed certificate is written to the Kubernetes Secret named
`tls.certManager.secretName` (default: `clock-server-tls`). If the Secret is mounted
into the pod, the updated files are available to the server. To pick up the new
certificate without downtime, perform a rolling restart (see below).

### Verification

Check the certificate status:

```sh
kubectl get certificate clock-server -n <namespace>
kubectl describe certificate clock-server -n <namespace>
```

Expected `Ready` condition:

```
Status:
  Conditions:
    Reason:  Ready
    Status:  True
    Type:    Ready
```

Check the secret was updated:

```sh
kubectl get secret clock-server-tls -n <namespace> -o jsonpath='{.data.tls\.crt}' \
  | base64 -d | openssl x509 -noout -dates
```

---

## Manual Certificate Rotation

Use this procedure when cert-manager is not available or when a certificate must be
replaced out-of-band (e.g., CA compromise, emergency re-key).

### Step 1: Obtain the new certificate and key

Obtain a new certificate (`tls.crt`) and private key (`tls.key`) from your CA or
generate a self-signed certificate for testing:

```sh
openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt \
  -days 90 -nodes -subj "/CN=clock-server"
```

### Step 2: Update the Kubernetes Secret

```sh
kubectl create secret tls clock-server-tls \
  --cert=tls.crt \
  --key=tls.key \
  --namespace=<namespace> \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Step 3: Trigger a rolling restart

clock-server reads the certificate files at startup. A rolling restart is required to
pick up the updated secret.

```sh
kubectl rollout restart deployment/clock-server -n <namespace>
```

Monitor the rollout:

```sh
kubectl rollout status deployment/clock-server -n <namespace>
```

### Step 4: Verify the new certificate is in use

Check server logs for the absence of expiry warnings:

```sh
kubectl logs -l app.kubernetes.io/name=clock-server -n <namespace> | grep -i tls
```

Inspect the certificate served by the running pod (replace `<pod>` and `<namespace>`):

```sh
kubectl exec <pod> -n <namespace> -- \
  openssl s_client -connect localhost:8080 -servername clock-server </dev/null 2>/dev/null \
  | openssl x509 -noout -dates
```

---

## cert-manager Configuration Reference

The following `values.yaml` fields control cert-manager integration:

| Field | Default | Description |
|---|---|---|
| `tls.certManager.enabled` | `false` | Enable/disable the `Certificate` resource |
| `tls.certManager.issuerRef.name` | `letsencrypt-prod` | Name of the Issuer or ClusterIssuer |
| `tls.certManager.issuerRef.kind` | `ClusterIssuer` | Resource kind (`Issuer` or `ClusterIssuer`) |
| `tls.certManager.duration` | `2160h` (90 days) | Requested certificate lifetime |
| `tls.certManager.renewBefore` | `720h` (30 days) | How early cert-manager renews the certificate |
| `tls.certManager.secretName` | `clock-server-tls` | Name of the TLS Secret cert-manager writes |

---

## Related

- [cert-manager Certificate API reference](https://cert-manager.io/docs/reference/api-docs/#cert-manager.io/v1.Certificate)
- [Audit log retention runbook](audit-log-retention.md)
