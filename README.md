# tln-db-k8s-operator

A Kubernetes operator for deploying and managing [tln-db](https://github.com/opentalon/tln-db)
— a single-node, bbolt-backed gRPC/HTTP database. Built with Kubebuilder /
controller-runtime, mirroring the conventions of
[`k8s-operator`](https://github.com/opentalon/k8s-operator).

The operator introduces a `TlnDB` custom resource and reconciles it into a
StatefulSet, Services, a rendered config ConfigMap, RBAC, and — optionally — an
Ingress, NetworkPolicy, ServiceMonitor, PodDisruptionBudget, and
HorizontalPodAutoscaler.

## Features

- **Persistent storage** via per-pod `VolumeClaimTemplates`, an existing PVC, or
  an ephemeral `emptyDir`.
- **ConfigMaps & Secrets**: a rendered `config.yaml` (or an external ConfigMap via
  `spec.configFrom`), plus arbitrary `env` / `envFrom` passthrough so Secrets and
  ConfigMaps can drive the server (`TLNDB_*`).
- **Metrics**: tln-db exposes Prometheus metrics at `/metrics`; the operator
  wires up the port and an optional `ServiceMonitor`.
- **Health probes**: HTTP `GET /v1/health` liveness/readiness/startup probes.
- **Config-driven rollouts**: a config-hash annotation rolls pods when config changes.

> **Note:** in `mode: standalone`, `spec.replicas > 1` creates *independent*
> databases (each pod owns its own volume) — it is **not** an HA cluster.

## Replicated mode

Set `spec.mode: replicated` to run a single-writer **leader** plus N read-only
**follower** replicas that stream the leader's op-log (async read-replicas):

```yaml
spec:
  mode: replicated
  replication:
    readReplicas: 2
    oplogRetention: 100000
```

The operator provisions a `-leader` StatefulSet (1 writer) and a `-follower`
StatefulSet (N replicas, each with its own PVC), and two Services:

- `<name>` — writes, targets the leader only.
- `<name>-read` — load-balanced reads across followers (falls back to the leader
  when `readReplicas: 0`).

Followers bootstrap from the leader's snapshot then tail its op-log; they reject
writes. This is async replication (warm standbys / read scaling), **not**
linearizable HA with automatic failover.

## Quick start

```sh
# Install CRDs and deploy the operator.
make install
make deploy IMG=ghcr.io/opentalon/tln-db-operator:latest

# Create an instance.
kubectl apply -f config/samples/db_v1alpha1_tlndb.yaml

# Inspect.
kubectl get tlndbs        # short name: tdb
kubectl describe tdb tlndb-sample
```

Or install everything from a single manifest / Helm (published on release):

```sh
kubectl apply -f https://github.com/opentalon/tln-db-k8s-operator/releases/latest/download/tln-db-operator.install.yaml
helm install tln-db-operator oci://ghcr.io/opentalon/charts/tln-db-operator
```

## Example

```yaml
apiVersion: db.tlndb.io/v1alpha1
kind: TlnDB
metadata:
  name: tlndb-sample
spec:
  image:
    repository: ghcr.io/opentalon/tln-db
    tag: latest
  config:
    tcp: ":9899"
    http: ":8080"
  envFrom:
    - secretRef: { name: tlndb-secrets }
  storage:
    persistence:
      enabled: true
      size: 1Gi
  observability:
    metrics:
      enabled: true
      serviceMonitor: { enabled: true }
```

## Development

```sh
make manifests generate fmt vet   # regenerate CRDs/RBAC/deepcopy
make test                         # unit tests (+ envtest assets)
make run                          # run the controller against the current kubecontext
make sync-chart-crds              # copy generated CRDs into the Helm chart
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
