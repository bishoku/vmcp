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

- Policies also cover content-bearing resource/prompt and negotiated client-request paths. Routing identifiers remain stable.
- JSON keys and numeric values are inspected under deterministic budgets; sensitive keys/numbers or uninspectable binary content can be denied rather than silently forwarded. Image/audio/blob content fails closed under an active inspection policy.
- Error data is inspected; operational upstream log data is omitted. Regex masking does not prevent prompt injection or cover Docker daemon logs from stdio workers.

See [security operations](SECURITY_OPERATIONS.md) for coverage and [the manual demo](../benchmark/README.md#5-pii-policy-quota-authentication-and-ui-checks) for synthetic PII tests.
