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

## F02 — Reliable interaction and frontend foundations (planned)

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

## F03 — Guided connection journey and topology (planned)

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

## F04 — Operations and product polish (planned)

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
