# clock-server Helm Chart

This chart deploys the clock-server application to a Kubernetes cluster.

## NetworkPolicy

### Overview

The chart can optionally create a Kubernetes `NetworkPolicy` resource that restricts
network traffic to and from clock-server pods. When enabled, the policy:

- **Ingress**: allows traffic only from the ingress controller namespace (default: `traefik`).
- **Egress**: allows only the following outbound connections:
  - DNS (UDP and TCP port 53) — required for Kubernetes service discovery.
  - MQTT broker on the configured port (default: 8883 for MQTTS).
  - REST downstream on the configured port (default: 443 for HTTPS).
- All other ingress and egress is denied implicitly.

NetworkPolicy is **additive** — enabling it will not break existing connectivity
as long as the egress ports match the actual downstream service ports you have configured.

### Prerequisite: CNI support

NetworkPolicy enforcement requires a Container Network Interface (CNI) plugin that
supports it. Supported CNIs include:

- [Calico](https://docs.tigera.io/calico/latest/network-policy/)
- [Cilium](https://docs.cilium.io/en/stable/network/kubernetes/policy/)
- [Weave Net](https://www.weave.works/docs/net/latest/kubernetes/kube-addon/)
- [Antrea](https://antrea.io/docs/main/docs/network-policy/)

**Note:** Flannel does **not** enforce NetworkPolicy by default.

### How to enable

Pass `--set networkPolicy.enabled=true` at install or upgrade time:

```shell
helm upgrade --install clock-server ./helm/clock-server \
  --set networkPolicy.enabled=true
```

Or add it to your `values.yaml` override file:

```yaml
networkPolicy:
  enabled: true
```

### Configuration

| Value | Default | Description |
|---|---|---|
| `networkPolicy.enabled` | `false` | Create the NetworkPolicy resource |
| `networkPolicy.ingressNamespaceSelector` | `{matchLabels: {kubernetes.io/metadata.name: traefik}}` | Namespace selector for the ingress controller |
| `networkPolicy.mqttEgressPort` | `8883` | TCP port for MQTT broker (use 8883 for MQTTS, 1883 for plain MQTT) |
| `networkPolicy.restEgressPort` | `443` | TCP port for REST downstream (use 443 for HTTPS) |

If your ingress controller runs in a namespace other than `traefik`, override the selector:

```shell
helm upgrade --install clock-server ./helm/clock-server \
  --set networkPolicy.enabled=true \
  --set networkPolicy.ingressNamespaceSelector.matchLabels."kubernetes\.io/metadata\.name"=ingress-nginx
```

### How to verify

After enabling NetworkPolicy, confirm the resource was created:

```shell
kubectl describe networkpolicy -n <namespace>
```

Check that the policy shows the expected ingress and egress rules. You can also
confirm pod-level enforcement by attempting a connection that should be blocked and
verifying it times out.
