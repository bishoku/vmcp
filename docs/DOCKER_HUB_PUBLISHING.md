# Publishing versioned Docker Hub images

The workflow `.github/workflows/docker-publish.yml` publishes application images
under the Docker Hub namespace **barishoku** when you push a version tag such as
`v0.0.1`. It first calls the existing CI workflow against the tagged commit.
No image is published unless all verification jobs succeed.

## Images

Every image supports `linux/amd64` and `linux/arm64`:

- `barishoku/vmcp-web:v0.0.1`
- `barishoku/vmcp-control-plane:v0.0.1`
- `barishoku/vmcp-data-plane:v0.0.1`
- `barishoku/vmcp-migrate:v0.0.1`
- `barishoku/vmcp-audit-exporter:v0.0.1`
- `barishoku/vmcp-audit-exporter-kafka:v0.0.1`
- `barishoku/vmcp-audit-exporter-rabbitmq:v0.0.1`
- `barishoku/vmcp-tenant-router:v0.0.1`
- `barishoku/vmcp-rotate-secrets:v0.0.1`

The first four serve the standard Helm installation. Enable exporters or the
router only when needed. The rotation image is an operational utility; the Helm
chart does not run rotation automatically. Test fixtures and third-party databases
or brokers are not published by this workflow.

Only the exact Git tag is pushed as an image tag. There is no `latest`, major, or
minor alias to move accidentally. Prerelease tags such as `v0.0.1-rc.1` work too.
Version components must not have leading zeroes; build-metadata suffixes containing
`+` are excluded because Docker tags do not support that character. Never move a
published Git tag. Registry version tags can still be overwritten on a rerun unless
you configure Docker Hub immutable-tag rules; deploy by digest for a stable identity.

## One-time Docker Hub setup

Sign in as `barishoku` and set the default repository privacy to **Public** for
this public project. Docker Hub creates missing repositories on the first push;
creating the nine repositories above manually is optional if you want to choose
their visibility individually. See [Docker Hub privacy settings](https://docs.docker.com/docker-hub/settings/).
No Docker Hub automated-build integration or GitHub account linking is required.

Create a personal access token through Docker Home → avatar → **Account settings**
→ **Personal access tokens** → **Generate new token**. Name it `vmcp-github-actions`,
choose an expiration, and select **Repo Read & Write** (or the equivalent Read &
Write label). Read-only cannot publish; delete permissions are unnecessary. Copy
the token when it is shown and keep it out of Git.
See [Docker's token creation guide](https://docs.docker.com/security/access-tokens/personal-access-tokens/)
and [permission reference](https://docs.docker.com/security/access-tokens/reference/).

For this personal namespace the workflow uses a PAT. Organization-managed OIDC
connections are a separate Docker authentication option; changing to an
organization namespace would require updating the workflow's authentication and
image references. See [Docker authentication options](https://docs.docker.com/security/access-tokens/).

## GitHub repository secret

In `bishoku/vmcp`, open **Settings → Secrets and variables → Actions → New repository
secret** and create:

- Name: `DOCKERHUB_TOKEN`
- Value: the Docker Hub personal access token you just generated.

The username is already set to `barishoku` in the workflow. You do not need a
Docker Hub password secret or a separate username secret. Rotate the token before
expiration by replacing this GitHub secret. The token is passed only to Docker's
login action, never to a Dockerfile build argument.

## Publish a version

Commit and push the workflow and application changes first. After checking the
commit you intend to publish:

```sh
git switch main
git pull --ff-only
git tag -a v0.0.1 -m "vMCP v0.0.1"
git push origin v0.0.1
```

Open **Actions → publish-docker-hub**. The run validates the tag, reruns core,
frontend and Helm CI for that commit, then builds/pushes the nine images with a
maximum of three concurrent publishing jobs. Multi-platform Go binaries are
cross-compiled on the native builder; the web assets also build natively.
The workflow attaches build provenance, SBOMs and source/version labels.

After successful image publication, `publish-github-release` creates a public
GitHub Release with deployment assets. The existing `release-artifacts` workflow
also runs on version tags and uploads standalone binaries as Actions artifacts;
those binaries are not attached to the public release. Neither workflow deploys a
Kubernetes application. Pushing a version tag does publish images; ordinary pushes
to `main` only run CI/wiki workflows.

If a matrix job fails, some other images may already have been pushed. A complete
image set is ready only after all nine publishing jobs and the `helm-values` job
succeed; the public GitHub Release appears after its separate packaging job succeeds. Re-run failed jobs for transient errors. Fix source defects with a new
version tag rather than moving the old tag. Image publication is not atomic across
Docker Hub repositories.

## Use the images with Helm

After successful publication, open [GitHub Releases](https://github.com/bishoku/vmcp/releases)
and download `helm-images.yaml`, `images.json`, the `vmcp-<chart-version>.tgz` chart
and `SHA256SUMS` for your version. Public release assets require no GitHub login
and do not expire like Actions artifacts. The Actions artifact
**docker-images-v0.0.1** remains available for troubleshooting.
It contains:

- `helm-images.yaml`: immutable multi-platform manifest references for web, both
  planes, migrations, router and exporters.
- `images.json`: version, image repository and digest for all nine images,
  including the secret rotation utility.

Keep your existing deployment configuration and layer the image overlay last:

```sh
helm repo add bitnami https://charts.bitnami.com/bitnami --force-update
helm dependency build deploy/helm/vmcp
helm upgrade --install vmcp deploy/helm/vmcp \
  --namespace vmcp --create-namespace \
  -f my-production-values.yaml \
  -f helm-images.yaml \
  --wait --wait-for-jobs --timeout 15m
```

The overlay changes image references only: it does not enable optional exporters
or the router, create credentials, or configure external infrastructure. The chart
requires digests by default; a plain `:v0.0.1` image reference will be rejected.
Public repositories do not require image pull credentials. For private repositories,
create a Kubernetes registry pull Secret using a separate read-only token and set
`imagePullSecrets`; do not distribute the publishing token to cluster workloads.

You can also inspect the registry directly:

```sh
docker buildx imagetools inspect barishoku/vmcp-data-plane:v0.0.1
docker pull barishoku/vmcp-data-plane:v0.0.1
```

See the [Helm deployment guide](../deploy/helm/vmcp/README.md) for configuration,
TLS, database roles, stateful dependencies and upgrade ordering.

## Verification and maintenance

```sh
python3 -m unittest discover -s scripts -p 'test_prepare_image_release.py'
```

Release-manifest tests reject incomplete image sets, mixed versions, duplicate
images, unexpected repositories and malformed digests. Docker action releases are
pinned by commit; update those pins deliberately. `scripts/prepare-image-release.py`
and the workflow namespace currently target `barishoku` together: update both if
moving registries or accounts. No Docker Hub credentials are committed to this repo.

## Public GitHub Releases and existing versions

`.github/workflows/github-release.yml` runs when `publish-docker-hub` completes
successfully. It validates the source repository, workflow, tag, commit and image
inventory, then packages the exact tagged chart with its locked dependencies.
The packaged chart's `appVersion` is the application tag; its chart `version`
remains the version declared in `Chart.yaml`. Both versions are independent.

Assets are uploaded to a draft before it becomes public. Tags containing a
prerelease suffix produce GitHub prereleases. Reruns can repair an incomplete
draft, but leave an already public release unchanged. A failed release packaging
job does not rebuild or repush Docker images. Retry it after resolving the cause.
No additional token is needed: the release job uses `GITHUB_TOKEN` with
`contents: write` and `actions: read`. Release creation uses the verified existing
tag without an explicit `target_commitish`: supplying an older commit with workflow
changes can trigger a workflow-write permission check that `GITHUB_TOKEN` cannot
satisfy. See [GitHub release API permissions](https://docs.github.com/en/rest/releases/releases#create-a-release).

To publish the already uploaded **v0.0.2** images after this workflow reaches
`main`, select **Actions → publish-github-release → Run workflow**, choose `main`,
and enter source run ID **38241826020**. This reuses the successful image artifact
and packages the chart from its original commit; it does not move the version tag.
Manual runs require an unexpired artifact from a successful version-tag Docker
publishing run. Future versions publish their GitHub Release automatically once
this workflow exists on the default branch.

Install directly from downloaded release assets without cloning the repository
or fetching chart dependencies separately:

```sh
sha256sum -c SHA256SUMS
helm upgrade --install vmcp ./vmcp-0.2.0.tgz \
  --namespace vmcp --create-namespace \
  -f my-production-values.yaml -f helm-images.yaml \
  --wait --wait-for-jobs --timeout 15m
```

Replace the chart archive name with the asset in your selected release. Prepare
infrastructure, Secrets, DNS, TLS and network policies using the chart guide first.
The release does not provision external infrastructure or enable optional workloads.
