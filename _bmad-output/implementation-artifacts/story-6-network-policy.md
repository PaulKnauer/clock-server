# Story 1.6: Add Kubernetes NetworkPolicy for Egress Restriction

Status: completed

## Story

As a platform engineer,
I want the clock-server pod's network egress restricted to only its required downstream services,
so that a compromised pod cannot reach arbitrary cluster resources.

## Acceptance Criteria

1. Helm template `templates/networkpolicy.yaml` added, enabled via `networkPolicy.enabled` (default `false`)
2. Default policy when enabled: deny all ingress except from ingress controller namespace; deny all egress except DNS (UDP/TCP 53), MQTT broker port, and REST downstream port
3. `values.yaml` documents `networkPolicy.mqttEgressPort`, `networkPolicy.restEgressPort`, `networkPolicy.ingressNamespaceSelector`
4. Helm lint passes with `networkPolicy.enabled: true` and with `networkPolicy.enabled: false`
5. Chart README documents how to enable NetworkPolicy and required pod/namespace selector labels

## Tasks / Subtasks

- [ ] Task 1: Create NetworkPolicy Helm template (AC: 1, 2, 3)
  - [ ] Create `helm/clock-server/templates/networkpolicy.yaml`
  - [ ] Guard entire resource with `{{- if .Values.networkPolicy.enabled }}`
  - [ ] Ingress rule: allow from `namespaceSelector` matching ingress controller namespace (configurable via `networkPolicy.ingressNamespaceSelector`)
  - [ ] Egress rules:
    - DNS: allow UDP + TCP port 53 to any destination (required for service discovery)
    - MQTT: allow TCP to `networkPolicy.mqttEgressPort` (default 1883/8883)
    - REST: allow TCP to `networkPolicy.restEgressPort` (default 443)
  - [ ] All other ingress and egress denied implicitly by `policyTypes: [Ingress, Egress]`

- [ ] Task 2: Update values.yaml (AC: 3)
  - [ ] Add `networkPolicy` block:
    ```yaml
    networkPolicy:
      enabled: false
      ingressNamespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ingress-nginx  # or traefik namespace
      mqttEgressPort: 8883   # use 8883 for MQTTS, 1883 for plain MQTT
      restEgressPort: 443
    ```
  - [ ] Add inline comments explaining each field

- [ ] Task 3: Helm lint validation (AC: 4)
  - [ ] Run `helm lint helm/clock-server` with `--set networkPolicy.enabled=true`
  - [ ] Run `helm lint helm/clock-server` with default values (networkPolicy disabled)
  - [ ] Both must pass without warnings

- [ ] Task 4: Documentation (AC: 5)
  - [ ] Update `helm/clock-server/README.md` with NetworkPolicy section
  - [ ] Document prerequisite: cluster must have a CNI that supports NetworkPolicy (Calico, Cilium, etc.)
  - [ ] Document how to verify policy is active: `kubectl describe networkpolicy -n <namespace>`
  - [ ] Note: NetworkPolicy is additive — enabling it won't break existing connectivity if egress ports are correct

## Dev Notes

- NetworkPolicy is purely declarative Kubernetes YAML — no Go code changes required
- The `policyTypes` field must include both `Ingress` and `Egress` to enforce both directions; omitting `Egress` means egress is unrestricted
- DNS egress (port 53) is easy to forget and will silently break service discovery if missing — always include it
- Use `podSelector: {}` in the NetworkPolicy `spec` to target all pods in the namespace (or scope to clock-server pods via `app: clock-server` label)
- For the ingress rule, if the cluster uses Traefik in its own namespace, the `namespaceSelector` should match that namespace's label
- The `helm lint` check is the primary test — template rendering correctness validates the YAML structure

### Project Structure Notes

- New: `helm/clock-server/templates/networkpolicy.yaml`
- Modified: `helm/clock-server/values.yaml`
- Modified: `helm/clock-server/README.md` (or create if missing)

### References

- [Source: helm/clock-server/values.yaml] — Existing Helm values structure and conventions
- [Source: helm/clock-server/templates/deployment.yaml] — Label selectors used (needed to match NetworkPolicy podSelector)
- Kubernetes NetworkPolicy spec: https://kubernetes.io/docs/concepts/services-networking/network-policies/

## Dev Agent Record

### Agent Model Used

claude-sonnet-4-6

### Debug Log References

### Completion Notes List

### File List

- New: `helm/clock-server/templates/networkpolicy.yaml`
- Modified: `helm/clock-server/values.yaml`
- New: `helm/clock-server/README.md`
