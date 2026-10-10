# Upstream TLS (self-signed certificates, private CAs, mTLS)

Internal MCP servers often use certificates from a private CA or self-signed ones. Give the upstream a `tls` block
instead of weakening verification:

```json
{
  "tls": {
    "caPem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
    "serverName": "mcp.internal.example",
    "clientCertPem": "-----BEGIN CERTIFICATE-----...",
    "clientKeyPem": "-----BEGIN EC PRIVATE KEY-----..."
  }
}
```

| Field | Behaviour |
|---|---|
| `caPem` | PEM bundle (up to 10 certificates, 64 KiB). It becomes the **only** trusted root **for this upstream**, not a global trust store. A self-signed server certificate can be used directly as the CA |
| `serverName` | Host name used for SNI and certificate verification. Needed when you connect by IP or an alias that is not in the certificate |
| `clientCertPem` + `clientKeyPem` | Client certificate for mutual TLS. Must be a valid pair |

Applies everywhere vMCP connects: tool discovery, connection tests, health checks and agent sessions.

## Rules

- **There is no "skip verification" option**, by design.
- `clientKeyPem` is encrypted at rest, **write-only**, and never returned or exported. If you omit it while keeping the
  same certificate, the stored key is kept; clearing `clientCertPem` also clears the key.
- `PUT`/`POST` replace the whole block (omit it to keep the stored one); `PATCH` merges per field and `""` clears a field.
- Export/apply carry `caPem`, `serverName` and `clientCertPem`, never the key.
- The UI shows each CA certificate's subject and expiry (and flags expired ones). Renew certificates before they expire:
  connections fail once they do.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `x509: certificate signed by unknown authority` | Missing or wrong `caPem` |
| `x509: certificate is valid for X, not Y` | Connecting by IP/alias: set `serverName` to a name in the certificate |
| `remote error: tls: certificate required` | Server needs mTLS: add the client certificate and key |
| Connection test fails for an mTLS upstream in the UI but the stored upstream works | The test can only use a key you pasted in this session; stored keys cannot be read back |
