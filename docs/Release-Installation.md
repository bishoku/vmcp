# Install a published release

Use a published release to install versioned container images and a packaged Helm
chart without building vMCP. You do not need Docker Hub publishing credentials,
GitHub Actions access or a repository publishing token.

For a disposable development installation with dependencies and generated secrets,
use [Kubernetes quickstart](HELM_QUICKSTART.md). For production, prepare infrastructure
and configuration using [Production deployment](PRODUCTION_DEPLOYMENT.md) and the
[Helm chart guide](../deploy/helm/vmcp/README.md).

## Download a version

Choose a version from [GitHub Releases](https://github.com/bishoku/vmcp/releases).
Download these assets from the same release:

- `vmcp-<chart-version>.tgz`: packaged chart, including locked dependencies.
- `helm-images.yaml`: digest-pinned application image references.
- `images.json`: application version and image inventory.
- `SHA256SUMS`: checksums for the release assets.

The application tag and chart version are independent. Use the archive name from
your selected release. Public releases and public images do not require login.
Before installation, verify the downloaded files against `SHA256SUMS`, for example
with `sha256sum -c SHA256SUMS` (or `shasum -a 256 -c SHA256SUMS` on macOS).
Download all listed assets if using a full-manifest check; missing files fail verification.

## Prepare configuration

You need a Kubernetes cluster, Helm and kubectl, database connectivity, provisioned
Secrets and appropriate permissions. For production, also prepare DNS, TLS, ingress,
backups and approved network destinations. See the chart guide for supported
versions and prerequisite database roles.

Create `my-production-values.yaml` using the
[production example](../deploy/helm/vmcp/examples/production.yaml), replacing all
placeholders. Keep secret values out of version control. The image overlay selects
images only; it does not provision external infrastructure or enable optional exporters.

## Install

From the directory containing the downloaded assets and your values file:

```sh
# Replace the archive name with the chart asset from your selected release.
helm upgrade --install vmcp ./vmcp-<chart-version>.tgz \
  --namespace vmcp --create-namespace \
  -f my-production-values.yaml -f helm-images.yaml \
  --wait --wait-for-jobs --timeout 15m
kubectl -n vmcp get pods,jobs
```

Use the intended cluster context explicitly or select it before running the commands.
The overlay is applied last so its immutable image digests take precedence.
Configure `runtime.publicGatewayURL` to the browser/client-reachable gateway URL.
The chart manages runtime UI configuration; you do not need to rebuild the web image.

Open the configured console URL and sign in using your configured admin token or
SSO. Follow [Guided setup](Connections-and-Setup.md), then verify a virtual server
using a separate agent credential. An installation that is ready is not yet a
configured agent connection.

## Upgrade and recover

Select the target release, review its changes and migration SQL, and back up the
database and encryption recovery keys. Download a matching chart/image overlay,
then run the installation command with your existing values. The chart coordinates
migrations; prerequisite changes may need staging before the upgrade.

Helm rollback does not reverse database migrations. Follow
[upgrade and rollback guidance](PRODUCTION_DEPLOYMENT.md#upgrade-rollback-and-support)
and [encryption recovery](SECURITY_OPERATIONS.md#encryption-upgrades-and-recovery).
Preserve your values and the exact image inventory for each deployment.

## Verify image signatures

New releases produced by the hardened publishing workflow are signed with Sigstore
Cosign using GitHub Actions OIDC. Older releases may not have these signatures.
Install Cosign from its official distribution and verify all image digests before
running Helm. From the downloaded release directory, with `jq` available:

```bash
set -euo pipefail
version=$(jq -r '.version' images.json)
jq -r '.images[] | .image + "@" + .digest' images.json | while IFS= read -r image; do
  cosign verify \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity "https://github.com/bishoku/vmcp/.github/workflows/docker-publish.yml@refs/tags/$version" \
    "$image" || exit 1
done
```

Require the expected repository, workflow and version identity; a signature from
an arbitrary signer is insufficient. A missing signature or verification failure
must stop installation. The chart itself does not install an admission controller
or enforce signatures inside the cluster; organizations can additionally enforce
this identity with their existing image admission policy.
