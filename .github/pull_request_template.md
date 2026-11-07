## Related issue

Refs #<!-- issue number; use Closes only when all acceptance criteria are complete -->

## What and why

## How it was verified
- [ ] `go test -race ./...` (with `VMCP_TEST_DATABASE_URL`)
- [ ] `npm run lint && npm run build` (if the UI changed)
- [ ] `api/openapi.yaml` updated and linted (if the API changed)
- [ ] Docs in `docs/` updated

## Not verified / known gaps

## Merge and release
- [ ] Required checks pass on the current PR head; no direct push to main or protection bypass
- [ ] Version/release notes included when applicable; tags are created only from the verified merged revision
