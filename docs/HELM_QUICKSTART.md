# Try vMCP on Kubernetes

The quickstart installs a small trial stack into your selected Kubernetes cluster:
PostgreSQL, authenticated Redis, migrations, the console, one control plane and two
data planes. Kafka, RabbitMQ, independent audit exporters and monitoring integration
are optional. It creates two Helm releases: `<release>` and `<release>-infra`.

This is a trial installer, not a production configuration. Internal connections
are plaintext, infrastructure has one replica, runtime database roles have broad
DML permissions, private upstream address ranges are allowed, and application
NetworkPolicies are disabled. Use an isolated development cluster or namespace.
For production, follow [Production deployment](PRODUCTION_DEPLOYMENT.md).

## Prerequisites

- A clone of this repository containing `deploy/helm/vmcp/quickstart.sh` and its
  adjacent `quickstart/` directory. The published v0.0.2 source predates this script;
  use the current main branch to try that application release.
- Bash, Python 3.9+, kubectl, Helm 3.17+, and Kubernetes 1.27+.
- Permission to create namespace-scoped workloads, Secrets, PVCs and Helm releases,
  and read nodes, cluster version and StorageClasses. Monitoring also reads CRDs.
- A default StorageClass, or `--storage-class NAME`. `--ephemeral` avoids PVCs but
  loses all dependency data when their pods are replaced.
- Network access to GitHub Releases and Docker Hub. Allow roughly 2 CPUs/4 GiB
  of spare capacity for the base trial, and additional capacity for brokers.
  Actual scheduling needs depend on the cluster and enabled components.

The installer prints the selected context and passes it explicitly to every
cluster command. It does not switch your kubeconfig context or create a cluster.

## Start

From the repository root:

```sh
./deploy/helm/vmcp/quickstart.sh --help
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT --version v0.0.2
```

It downloads the selected release's chart, checksums and image inventory, checks
asset checksums and image digests, creates/reuses credentials, installs dependency
workloads, creates database roles, runs migrations, grants runtime/export privileges,
creates broker destinations when selected, and installs vMCP. Helm waits for readiness.
The packaged application chart includes its locked dependencies; the quickstart
uses its own small infrastructure chart with pinned official images instead of
turning on the production chart's optional Bitnami subcharts.

By default it keeps two localhost-only port forwards running:

- Console: `http://localhost:3300`
- Gateway: `http://localhost:8801`

The installer prints a command to retrieve the admin token from its Kubernetes
Secret. It does not print the token. Sign in using that token, register your HTTP
upstreams, create a virtual server, and issue an agent key from the console.
Gateway endpoints are `http://localhost:8801/mcp/v1/virtual/<slug>`.

Ctrl-C stops forwarding; the installed stack stays running. Use
`--no-port-forward` to exit after installation and print forwarding commands.
`--web-port` and `--gateway-port` select different local ports and update runtime
URLs. Internal cluster workloads should use the data-plane Service endpoint.

## Optional brokers and exporters

```sh
# Brokers only: Kafka topic and RabbitMQ queue are created, but no exporters run.
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT --kafka --rabbitmq

# Independent exporters: each flag also enables its corresponding broker.
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT \
  --exporter kafka --exporter rabbitmq --no-port-forward

# Your own HTTP receiver, reachable from cluster pods.
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT \
  --exporter http --http-export-url https://receiver.example.com/audit
```

Exporters use distinct consumers (`quickstart-http`, `quickstart-kafka`,
`quickstart-rabbitmq`) and a separate database LOGIN role. Kafka uses the
`vmcp-audit` topic; RabbitMQ uses a durable `vmcp-audit` queue through the default
exchange. Generate audit events by configuring or calling virtual servers, then
inspect broker messages and exporter logs. Kafka uses unauthenticated plaintext
inside the trial cluster; Redis and RabbitMQ use generated passwords but no TLS.
Avoid credentials embedded in command-line URLs; configure a suitable trial receiver.

`--monitoring` enables ServiceMonitors, Prometheus rules and dashboard ConfigMaps.
It requires existing Prometheus Operator CRDs, a Prometheus instance selecting the
resources, and a Grafana dashboard sidecar. It does **not** install Prometheus or
Grafana. Use `--monitoring-release NAME` to match your Prometheus selection label.

## Customize access and storage

```sh
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT \
  --namespace my-vmcp --release my-vmcp \
  --storage-class standard --data-plane-replicas 3 \
  --web-port 3301 --gateway-port 8802 --timeout 20m
```

For HTTP ingress, first provision DNS and an ingress controller. For example,
with MicroK8s, an existing `nginx` IngressClass, `microk8s-hostpath` storage and
wildcard DNS for `*.kube.local` pointing to your ingress controller:

```sh
./deploy/helm/vmcp/quickstart.sh --context microk8s \
  --storage-class microk8s-hostpath \
  --web-host vmcp.kube.local --gateway-host mcp.kube.local \
  --ingress-class nginx --exporter kafka --exporter rabbitmq \
  --no-port-forward --timeout 20m
```

This installs the base stack plus both brokers and their audit exporters. Open
`http://vmcp.kube.local`; gateway endpoints use
`http://mcp.kube.local/mcp/v1/virtual/<slug>`. Without `--tls-secret`, ingress has
no TLS configuration and nginx HTTPS redirects are disabled. Check your actual
IngressClass and StorageClass names with `kubectl get ingressclass,storageclass`.

For HTTPS ingress, also provision a TLS Secret in the target namespace before
running the installer, then supply `--tls-secret`:

```sh
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT \
  --web-host console.example.com --gateway-host mcp.example.com \
  --tls-secret vmcp-public-tls --ingress-class nginx
```

Ingress mode prints HTTP or HTTPS URLs according to `--tls-secret` and does not
start port forwards. The default
streaming annotations target nginx; other controllers may need manual adjustment.
TLS on ingress does not make the trial's internal infrastructure production ready.

Advanced/offline release selection accepts `--chart PATH --images-file PATH`.
The JSON inventory must match `--version` and contain all nine expected digest-pinned
images. For a local chart directory the installer restores locked dependencies;
that requires chart repository access. An archive already contains them. These
options skip release asset downloading/checksum verification; use trusted local files.
Cluster nodes still need access to the specified container images.

## Reruns, troubleshooting and cleanup

Rerun with the same namespace/release and the desired complete set of options.
Existing encryption keys, admin tokens and infrastructure passwords are reused.
Reruns are upgrades; omitted optional workloads are disabled. Persistent data stays
in PVCs, and credentials stay in `<release>-quickstart`. Preserve that Secret with
your database: losing the master key prevents decrypting existing upstream secrets.
Do not manually change database passwords or switch persistent/ephemeral storage
on an existing trial. Run only one installer per namespace/release at a time.

If an install fails, it retains resources for inspection:

```sh
kubectl --context YOUR_CONTEXT -n vmcp-demo get pods,jobs,pvc
kubectl --context YOUR_CONTEXT -n vmcp-demo get events --sort-by=.lastTimestamp
kubectl --context YOUR_CONTEXT -n vmcp-demo logs POD_NAME --all-containers
helm --kube-context YOUR_CONTEXT -n vmcp-demo list --all
```

Uninstall while preserving Secrets and PVCs:

```sh
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT --uninstall
```

Delete the trial's stored data and credentials explicitly:

```sh
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT --uninstall --purge
```

Use the same `--namespace` and `--release` used for installation. Neither command
deletes the namespace or cluster. PVC deletion follows your StorageClass reclaim
policy; it may delete the underlying volumes. Failed Helm/wait commands print resource status and events. No automatic rollback or destructive
cleanup runs on installation failure.
