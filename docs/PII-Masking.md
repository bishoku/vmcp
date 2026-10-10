# PII masking

Per virtual server, vMCP can replace sensitive values in tool **results** (and optionally **arguments**) before they leave
the gateway. Matches become `[REDACTED:<name>]`.

```json
{"pii": {
  "enabled": true,
  "detectors": ["email", "iban", "credit_card"],
  "customRules": [{"name": "employee_id", "pattern": "EMP-\\d{6}"}],
  "maskArguments": false
}}
```

| Field | Meaning |
|---|---|
| `enabled` | Master switch |
| `detectors` | Built-in detectors to use; empty means **all** |
| `customRules` | Up to 20 named regular expressions (RE2 syntax, max 500 characters, linear time) |
| `maskArguments` | Also mask arguments sent *to* the upstream (e.g. keep the model from sending a card number on) |

## Built-in detectors

`iban` (checksum-validated), `credit_card` (Luhn-validated), `tckn` (Turkish ID number, checksum-validated), `us_ssn`,
`email`, `phone`. Specific, validated types run first so a card number is not mistaken for a phone number.

## Notes

- Each audit event records the number of `redactions` applied.
- Pattern-based detection is best effort: it reduces exposure, it is not a guarantee. Do not rely on it as the only control
  for regulated data.
