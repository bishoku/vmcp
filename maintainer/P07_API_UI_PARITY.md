# API and UI parity (P07)

Management lists traverse every cursor, including tools. Resource read models
include `version`; the UI retains the version loaded into each edit draft and
sends it as quoted `If-Match` on updates, deletion, key/token rotation and
revocation. A 412 keeps the draft and asks the operator to reload. Omitted
`If-Match` remains compatible with existing API integrations. Administration
transactions take the configuration revision lock before resource locks, giving
GitOps revision checks and ordinary mutations a consistent lock order.

Settings exposes secret-free configuration export, JSON import, dry-run change
preview and apply. Prune requires an explicit confirmation. Editing the document
or prune option invalidates the preview. Apply sends the revision used at preview
and rejects intervening configuration mutations. Export reads configuration and
its revision in one repeatable-read, read-only database transaction. Imported
omitted headers, private keys and OAuth secrets retain existing stored values;
new upstreams still need operator-provided credentials where required. Discovery
uses the same primary/fallback URL sequence for refresh and declarative apply.

Virtual server selections retain missing upstream tools and display their orphan
state. Description drift remains visible. Audit supports a subject filter; admin
token creation accepts expiry and the table shows description and expiry.
`POST /api/v1/upstreams/{slug}/connection-test` requires write permission and uses
stored credentials without returning them, schemas, server descriptions or raw
errors. The UI distinguishes testing edited settings from testing stored ones.

OpenAPI models cookie authentication and its conditional CSRF header, optimistic
concurrency, nullable policy overrides and composed read/create schemas. Flattened
closed schemas retain unknown-property rejection while permitting declared fields.
`api/examples.json` contains captured synthetic test API responses, including
redacted one-time example secrets. It is validated with JSON Schema 2020-12.

Verification commands from `web/`:

```sh
npm test
npm run lint
npm run build
npm run validate:api
npx playwright install chromium
npm run test:browser
```

Browser tests mock the control-plane transport and exercise actual React flows;
Go integration tests use real PostgreSQL and fixture MCP upstreams for authorization,
concurrency, export/apply, stored authentication and orphan/pagination behavior.
No lint warnings are suppressed. The effect timers coalesce mount-time refreshes
and are cancelled on cleanup; request cancellation for already-started frontend
fetches remains a separate usability improvement.
