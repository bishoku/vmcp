# Tool policies

Each virtual server has a policy that decides which of its selected tools agents get:

```json
{"policy": {"default": "read_only", "tools": {"alpha__add": "allow", "crm__delete_customer": "deny"}}}
```

| Mode | Effect |
|---|---|
| `allow` | The tool is listed and callable |
| `deny` | The tool is hidden and not callable |
| `read_only` | Allowed **only if the operator sets `readOnly: true`** in the effective upstream/tool invocation policy |

`default` applies to tools without an entry in `tools` (keys are **exposed names**, i.e. aliases if set).

## Semantics

- Blocked tools are removed from `tools/list`, so agents cannot even see them.
- A call to a blocked tool is refused and recorded in the [audit log](Audit-Log.md) with status `denied`.
- `read_only` uses operator classification, not upstream hints. Unclassified tools are blocked. Classification and retry are independent; a repeatable operation may still write data. Restrict upstream credentials as well: a gateway label cannot prevent a malicious upstream from writing.
- Changes take effect on all data-plane nodes within moments, with no restart.
