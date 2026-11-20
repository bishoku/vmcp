# Install vMCP with Kustomize or Argo CD

Choose a [release](https://github.com/bishoku/vmcp/releases) that lists
`vmcp-kustomize-<tag>.tar.gz`. Older releases may only offer Helm. The bundle
contains ordinary Kubernetes resources rendered from that release's production
chart and signed-image digest inventory. Consumers need kubectl with Kustomize,
or an Argo CD Application using its built-in Kustomize support.

## Prepare the environment repository

Run these commands from the directory in your infrastructure repository that
will hold vMCP. Set VERSION to the exact release tag you selected. Use a fresh
download directory for each version.

```sh
set -eu
: "${VERSION:?Set VERSION to the selected release tag}"
archive="vmcp-kustomize-$VERSION.tar.gz"
download=$(mktemp -d)
gh release download "$VERSION" -R bishoku/vmcp -D "$download" -p "$archive" -p SHA256SUMS
(cd "$download" && awk -v f="$archive" '$2 == f {print; n++} END {if (n != 1) exit 1}' SHA256SUMS > selected.sum && sha256sum -c selected.sum)
mkdir -p upstream
tar -xzf "$download/$archive" -C upstream

# First install only. Keep these environment overlays in Git across upgrades.
cp -R "upstream/vmcp-kustomize-$VERSION/prod" prod
for phase in prerequisites migration app; do
  sed "s|../../base/|../../upstream/vmcp-kustomize-$VERSION/base/|" \
    "upstream/vmcp-kustomize-$VERSION/prod/$phase/kustomization.yaml" \
    > "prod/$phase/kustomization.yaml"
done
```

Commit the extracted release and your `prod/` overlays. Prepare the namespace,
external PostgreSQL database and roles, Redis, ingress controller, TLS and
Secrets before syncing. Follow the [production chart guide](../deploy/helm/vmcp/README.md)
for role grants and key generation. The bundle references these names in namespace
`vmcp`:

| Secret | Keys |
| --- | --- |
| vmcp-database | control-plane-url, data-plane-url, migration-url, listener-url |
| vmcp-encryption | master-key |
| vmcp-admin | token |
| vmcp-redis | url |
| vmcp-public-tls | tls.crt, tls.key |

The [production profile](../deploy/helm/vmcp/examples/production.yaml) uses
example hosts and dependency IPs. Add patches to `prod/prerequisites` for the
NetworkPolicy egress rules (PostgreSQL and Redis destinations), cluster DNS,
and ingress/monitoring selectors. In `prod/app`, set the Ingress hosts, class
and TLS hosts, the data-plane Deployment's VMCP_PUBLIC_URL and VMCP_ALLOWED_ORIGINS,
and the web ConfigMap's runtime-config.js gateway URL consistently. If changing
VMCP_TENANT_ID, set it on the migration Job and all workloads. Protect credentials
through your cluster's Secret management system.

For example, append this patch to `prod/app/kustomization.yaml` to set the
data-plane replica count in the standard profile:

```yaml
patches:
  - target:
      kind: Deployment
      name: vmcp-data-plane
    patch: |-
      - op: replace
        path: /spec/replicas
        value: 3
```

Review the complete result with `kubectl kustomize prod`. Optional Helm components
such as brokers or exporters require a separately rendered profile.

## Install and upgrade with kubectl

Create the namespace and provision the dependencies above first. Run the
following from the environment repository, for both first install and upgrade:

```sh
set -eu
kubectl -n vmcp apply -k prod/prerequisites
job=$(kubectl -n vmcp apply -k prod/migration -o name)
kubectl -n vmcp wait --for=condition=complete --timeout=15m "$job"
kubectl -n vmcp apply -k prod/app
kubectl -n vmcp rollout status deployment -l app.kubernetes.io/instance=vmcp --timeout=10m
```

For an upgrade, back up the database, download and verify the new bundle, and
change the three base references in your existing overlays to the new tag.
Review the rendered diff and schema compatibility with the running version.
Each tag has its own Job name. Reapplying a completed Job keeps its result.
If migration fails, inspect `kubectl -n vmcp describe "$job"` and
`kubectl -n vmcp logs "$job"`. Correct the cause and verify database state
before deleting a failed Job and retrying. The deployment step only runs
after success. Application rollback does not reverse database changes.

## Install and upgrade with Argo CD

Point one Application at the parent overlay. Replace the example repository,
revision and path with your own. This example targets an existing namespace
with prerequisites and Secrets provisioned as described above.

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: vmcp
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/YOUR-ORG/YOUR-INFRA.git
    targetRevision: main
    path: infra/vmcp/prod
  destination:
    server: https://kubernetes.default.svc
    namespace: vmcp
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - PruneLast=true
```

Apply the Application through your platform's normal bootstrap process. The
included overlays assign prerequisites to wave -2, migration to wave -1 and
application resources to wave 0. A full sync waits for the migration Job to
complete. A failed migration blocks the workload update. Ensure externally
managed Secrets exist before sync. A selective sync of individual Deployments
can bypass the ordering, so use a full Application sync for upgrades.

To upgrade, commit the new bundle and update all three references in one change.
Argo CD retains the completed Job across ordinary syncs. The Job has no TTL,
so self-heal will not rerun it due to TTL expiry. The new tag creates a fresh
Job and final pruning removes the superseded one after a healthy rollout.
See [Argo CD sync waves](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/).
