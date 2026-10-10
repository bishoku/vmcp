# Tool policies

Each virtual server has a policy that decides which of its selected tools agents get:

```json
{"policy": {"default": "read_only", "tools": {"alpha__add": "allow", "crm__delete_customer": "deny"}}}
```

| Mode | Effect |
|---|---|
| `allow` | The tool is listed and callable |
| `deny` | The tool is hidden and not callable |
| `read_only` | Allowed **only if the upstream declares `annotations.readOnlyHint: true`** for the tool |

`default` applies to tools without an entry in `tools` (keys are **exposed names**, i.e. aliases if set).

## Semantics

- Blocked tools are removed from `tools/list`, so agents cannot even see them.
- A call to a blocked tool is refused and recorded in the [audit log](Audit-Log.md) with status `denied`.
- `read_only` trusts the upstream's own annotation. A tool without the hint is treated as *not* read-only and blocked,
  which is the safe direction. If you do not trust an upstream's annotations, use explicit `allow`/`deny` entries.
- Changes take effect on all data-plane nodes within moments, with no restart.
