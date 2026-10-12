# Frontend implementation plan

The frontend review identified correctness, accessibility, workflow and operational
visibility gaps. Preserve the existing React/Vite/Tailwind foundation. Each phase
must pass its regression tests, lint, TypeScript/production build, OpenAPI
validation and browser tests before its implementation is committed. Do not mix
unfinished work from the next phase into a phase commit. Record verification and
remaining limitations below; passing frontend checks is not a production-readiness
certification.

## F01 — Correctness and accessible dialogs (complete)

- Replace guessed `:8080` connection URLs with the API endpoint path plus an
  explicitly configured public gateway base URL. Configure both Compose stacks
  and document Vite/Docker setup. If unconfigured, show the path and explain the
  missing configuration rather than advertise an invented URL.
- Keep tool policy overrides attached to upstream/tool identity while editing;
  serialize against the final exposed names. Alias edits, clearing aliases,
  temporary duplicates, swaps and removal must not accidentally grant access.
- Preserve exact existing agent-key expiry on unrelated edits. Allow explicit
  expiry changes with date/time and timezone context, including clearing expiry.
- Use native modal dialogs for forms and the upstream drawer: accessible names,
  initial focus, focus containment, Escape/backdrop dismissal where permitted,
  focus restoration, scrollable content and deliberate one-time-secret dismissal.
- Regression coverage: component tests for endpoint/copy, policy serialization,
  exact expiry preservation and explicit edits; real browser tests for dialog
  focus, keyboard dismissal, focus restoration and secret acknowledgement.

## F02 — Reliable interaction and frontend foundations (complete)

- Add dirty-draft guards for switching entities, navigation and dismissal; retain
  drafts after failed writes and provide actionable stale-version recovery.
- Make role capabilities available throughout the app; explain read-only access
  and hide/disable unavailable mutations without replacing API authorization.
- Standardize loading, empty, partial-error and mutation states; prevent duplicate
  submissions and preserve mutation errors through refresh.
- Cancel obsolete requests and prevent stale results replacing current filters.
  Load independent catalogs independently so one failure does not hide all data.
- Enable TypeScript strict checks, consolidate shared components/styles and split
  large page components around forms, capability selection and request hooks.
- Improve login/session expiry and clipboard error feedback. Prefer SSO; decide
  explicitly whether token login is memory-only or has an opt-in persistence
  policy. Never silently retry non-idempotent mutations.
- Tests: permission matrix, dirty-form decisions, stale responses, partial failures,
  expired sessions, duplicate submissions and failed clipboard writes.

## F03 — Guided connection journey and topology (complete)

- Add an onboarding path: register → test → discover → compose → grant access →
  connect → verify. Expose tools/resources/templates/prompts consistently.
- Give virtual servers focused Overview, Capabilities, Access, Protection, Connect
  and Activity views with deep links and clear saved/draft status.
- Add Connections topology from keys/bindings through virtual servers to upstreams;
  show capability counts, aliases, policies, missing objects and per-node health
  with timestamps. Selecting nodes/edges opens relevant details/configuration.
- Provide an accessible relationship table and filtering for larger installations.
- Distinguish configured permissions from observed traffic. Do not label a key as
  an active client. Begin with configuration and bounded recent audit activity;
  active-session counts and traffic aggregates require backend/API work first.
- Connect view: authoritative endpoint, credential placeholders, copyable client
  configuration and explicit verification. Tool invocation must be deliberate:
  do not automatically execute potentially destructive upstream tools.
- Tests: connection relationships, anonymous access, revoked/expired credentials,
  orphaned capabilities, permissions, large graphs and keyboard navigation.

## F04 — Operations and product polish (complete)

- Group audit admissions/outcomes by operation; provide detail drawers, copyable
  correlation IDs, server/identity/time filters, timezone labels and deep links.
  Preserve raw event access and distinguish incomplete work from confirmed errors.
- Add responsive navigation, editor layouts and intentional table overflow.
- Improve first-use/filtered empty states, searchable catalog selection, save/copy
  feedback, consistent status semantics and actionable health messages.
- Correct branding/title, accessible names, focus visibility and contrast; expand
  browser scenarios to real-stack smoke checks plus visual/accessibility checks.
- Set measurable acceptance criteria: no loss of drafts, no stale filter results,
  keyboard-complete primary flows, usable layouts at 375/768/1440px, documented
  data freshness, and successful end-to-end connection from a fresh sandbox.

## Verification ledger

F01 completed 2026-10-11:

- 18 unit/component tests passed, including 10 new regression cases.
- All 4 Chromium browser tests passed, including 3 new dialog scenarios.
- Agent-key expiry regressions also passed with `TZ=America/New_York`.
- Frontend lint, TypeScript/production build, OpenAPI validation (49 operations,
  6 captured examples), both Compose configuration checks and `git diff --check`
  passed.
- Browser regressions use mocked management APIs; they validate actual Chromium
  dialog/focus behavior but are not a substitute for the F04 real-stack smoke test.
- Public URL configuration is build-time; existing deployed web images must be
  rebuilt. This phase did not rebuild or redeploy the running manual-test stack.
- F02–F04 remain planned; dirty-draft protection, role-aware controls, the topology
  view, audit grouping and broader responsive/accessibility coverage are not
  implemented by F01.


F02 completed 2026-10-11:

- Shared draft baselines guard entity switching, dismissal, routing/Back, sign-out
  and unload. Failed writes retain their inputs; stale edits offer an explicit
  reload of the latest version. Pending operations block route departure.
- Role context and capabilities match backend permissions. Viewers retain list
  filtering, refresh and inspection; editors can import configuration, while
  admin token mutations remain admin-only.
- Shared cancellable read sources expose loading and partial errors without
  erasing mutation errors. Audit reads debounce filters, ignore stale responses,
  and protect paginated loading against duplicate requests/events. Catalog
  discovery preserves selections and offers retry for unavailable upstreams.
- Save/credential actions use synchronous operation latches; pending GitOps
  previews lock their document so an obsolete preview cannot authorize edits.
  No automatic mutation retries were introduced.
- TypeScript strict mode is enabled. Common styles, draft/request/operation hooks,
  clipboard feedback and stale recovery are shared; the upstream editor is split
  from its list and capability selection remains in CatalogPicker.
- Login tokens are memory-only, legacy persisted tokens are removed, and expired
  requests clear credentials. Token reauthentication preserves the mounted draft.
  SSO redirects necessarily reload the page; the UI warns users to copy changes.
- 35 unit/component tests passed (17 added regressions), covering role permissions,
  dirty decisions, duplicate saves, partial reads, cancellation, stale audit
  results, stale-version reload, session expiry, preview locking and clipboard
  failures. All 10 Chromium scenarios passed (6 added), including browser Back,
  viewer/editor/admin controls and draft retention after an expired write.
- Lint (no warnings), strict TypeScript/production build, OpenAPI validation
  (49 operations and 6 examples), and `git diff --check` passed.
- Browser scenarios still use mocked APIs. The running manual-test stack was not
  rebuilt. F03 topology/guided setup and F04 responsive polish, audit grouping,
  real-stack smoke and broader accessibility checks remain planned.


F03 completed 2026-10-11:

- Added Setup guide and Connections navigation. The seven-step guide links
  registration, explicit stored-credential testing, discovery, composition,
  grants, connection and verification without inferring successful tests.
- Saved virtual servers have deep-linked Overview, Capabilities, Access,
  Protection, Connect and Activity views. Editing remains a guarded draft with
  saved-version and unsaved-change status; node links select the relevant editor.
- Added configuration topology and an accessible, searchable relationship table.
  Inactive credentials, disabled servers, anonymous bypass, conditional bindings,
  missing objects/upstreams, aliases, policies and health timestamps remain visible.
  The focused diagram bounds each side to 8 relationships and tables expand in
  batches of 50; model regression covers 1,000 servers and 2,000 edges.
- Tools/resources/templates/prompts are shown in saved capability views and
  upstream discovery snapshots. Counts are selected configuration, not runtime
  execution grants. Partial inventory errors and data freshness are explicit.
- Connect uses the configured public gateway and copyable credential placeholders.
  Deliberate verification lazy-loads the pinned runtime MCP SDK, initializes,
  pings and lists advertised families only, using a separate memory-only gateway
  credential. No tool calls, resource reads, prompt execution or client feature
  permissions are involved. Requests/cancellation/cleanup are bounded.
- Added exact-origin CORS support to the demo reverse proxy, aligned with the
  existing data-plane Origin allowlist. Other deployments still configure their
  own proxy CORS or shared origin. Core protocol/auth behavior was unchanged.
- Activity shows at most 50 stored events from the last 15 minutes and links to a
  server-filtered audit log. No active-client counts or traffic aggregates exist.
- Fixed duplicate draft navigation confirmation handling exposed by deep links.
- 54 unit/component tests passed (19 F03 regressions), all 13 Chromium scenarios
  passed (3 F03 scenarios), and all 3 isolated Nginx CORS regressions passed.
  Lint (no warnings), strict TypeScript/production build, OpenAPI validation
  (49 operations / 6 examples), Compose configuration and `git diff --check` passed.
- Inspected a rendered desktop topology screenshot. Browser tests use mocked API
  and MCP responses; the SDK unit test exercises actual SDK framing. Full-stack,
  responsive and broader accessibility checks remain F04. No running demo images
  or services were rebuilt/restarted; rebuild web and reload its proxy to try F03.
- See [Connections and setup](../docs/Connections-and-Setup.md) for behavior, security
  boundaries, CORS configuration, pagination and verification limitations.


F04 completed 2026-10-11:

- Audit defaults to operation grouping by operation ID, with separate raw events,
  keyboard-accessible detail drawers and copyable operation/request/trace IDs.
  Uncorrelated events remain separate even when their request ID matches. Missing
  outcomes mean “outcome not loaded”, never an inferred error. Filtering and
  pagination may split a pair; the drawer preserves every loaded raw field.
- Server, key, subject, actor, status, operation and browser-local time filters
  have accessible names. Filter links retain supported query parameters and the
  drawer deep-links to an operation. Times identify the browser timezone; manual
  refresh and the last successful read timestamp make freshness explicit. Filter
  changes clear old results immediately and retain cancellation/sequence guards.
- Mobile navigation scrolls horizontally, editors stack at narrow widths, grid
  spans respond correctly and wide management tables scroll inside their cards.
  Populated primary views/editors and audit were checked at 375/768/1440px, with
  screenshots inspected for mobile editing and tablet topology.
- Catalog search matches upstream/kind/name/description without altering selected
  objects or falsely reporting filtered objects missing. First-use states explain
  the next step; upstream/server saves and copy actions provide success feedback.
  Copy success is tied to the copied value and announced to assistive technology.
  Shared status colors, actionable health guidance, vMCP title/favicon, named
  navigation/form controls and visible keyboard focus complete the polish.
- 58 unit/component tests passed (4 F04 additions), including operation grouping,
  missing-outcome semantics, catalog search, copy feedback and save feedback.
  All 21 mocked Chromium scenarios passed (8 F04 additions), covering responsive
  layouts, audit drawer/raw access/deep links, keyboard behavior and targeted
  theme contrast thresholds. Existing draft retention, permissions, session
  expiry and obsolete-response regressions continue to pass.
- A separate real Chromium smoke test passed against a fresh full Docker sandbox:
  PostgreSQL, Redis, Keycloak, both data planes, gateway, control plane, built UI
  and three synthetic upstreams. API setup registers/discovers a real upstream,
  composes a resource/prompt server and issues a key; the browser signs in and
  initializes, pings and lists its resource/prompt through the real gateway.
  The test never invokes a tool. Temporary containers, network, database volumes
  and test image tags are removed; the running manual demo was untouched.
- Lint, strict TypeScript/production build, OpenAPI validation (49 operations /
  6 examples), Compose configuration and whitespace checks passed. Vite reports
  a non-failing approximately 501 kB main-chunk advisory; further route splitting
  remains a performance follow-up rather than a raised warning threshold.
- Limits: targeted contrast/focus checks and screenshot inspection are not a full
  accessibility certification or automated pixel baseline. Live smoke uses token
  login and API fixture creation; SSO, UI-only provisioning and adverse-network
  journeys still need broader end-to-end coverage. Configuration/health remains
  snapshot data, not live traffic. See [Connections and setup](../docs/Connections-and-Setup.md)
  for operational semantics and repeatable test commands.
