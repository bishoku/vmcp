# Upstream detail editing (unreleased)

Release metadata: unreleased web UI fix after v2.1.1; no database migration,
API contract change, or release tag is introduced by this change.

- Upstream configuration, invocation defaults, local verification/approval
  services, tool policies and audit masking now share Edit upstream and a single
  Save/Cancel bar. Viewing a detail page no longer exposes security mutations.
- REST operations and discovered MCP tools link to one central policy editor.
  Effective policies show labeled requirements, with inherited defaults and
  tool-specific settings identified separately.
- Invocation security and audit masking use the same page width, spacing and
  disclosure groups as connection settings. Local masking rules have a saved
  summary outside edit mode; global and mandatory masking remain in effect.
- Saves still use independently revisioned endpoints. A failed save identifies
  completed sections, retains pending drafts and allows retry without resending
  completed sections. Cancel discards pending changes, not previously saved ones.
- Existing endpoint permissions, write-only credentials and conflict checks are
  retained. Audit-only editors can enter the shared edit mode without acquiring
  permission to edit connection settings. Virtual-server and global audit editors
  retain their existing workflows.
