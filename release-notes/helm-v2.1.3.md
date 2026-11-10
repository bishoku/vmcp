# Helm chart 2.1.3 — independent Omni model egress

Refs #65. Chart version: 2.1.3. Application version: v2.1.2.

- Add `omni.egressAllowCIDRs` and `omni.egressAllowHosts`, defaulting to empty lists.
- Render them only into Omni; never inherit the CP/DP runtime grants or expose
  model-only grants to CP/DP. Existing reserved environment-variable guards remain.
- Private embedding/rerank destinations require explicit CIDRs. If a host allowlist
  is configured, list every approved model host as well. NetworkPolicy remains separate.

This is a chart-only patch: no application code, new Docker images, API version
change or database migration. The image overlay and inventory are unchanged from
v2.1.2, retaining its immutable digests and existing signatures; no v2.1.3 image
publication or fresh image scan is claimed. The new chart is published under
`helm-v2.1.3`; the existing v2.1.2 release and tags remain unchanged.

Upgrade using the packaged chart and the supplied overlay:

```sh
helm upgrade vmcp-v2 ./vmcp-2.1.3.tgz --namespace vmcp-v2 \
  --reuse-values -f omni-egress.yaml -f helm-images.yaml \
  --atomic --wait --wait-for-jobs --timeout 15m
```

Populate `omni-egress.yaml` with your independent Omni grants before upgrading.
Helm automatic migration hooks still run. Rollback to chart 2.1.2 removes the
Omni grants; private models will again be unavailable unless configured separately.
Older application upgrades still require their release-specific preparation.
The legacy global `VMCP_APPROVAL_URL` approval hook remains removed.
