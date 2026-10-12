# Independent audit exporters

vMCP offers three separately deployed binaries:

- `audit-exporter`: HTTPS JSON POST.
- `audit-exporter-kafka`: Kafka producer.
- `audit-exporter-rabbitmq`: RabbitMQ AMQP 0-9-1 publisher.

They read persisted audit events directly from PostgreSQL using their own
connection pools (maximum two connections per process). They do not run inside
the data plane or control plane. Broker clients are imported only by the respective
exporter commands; no gateway configuration change or new migration is needed.
Deploy only the exporters your infrastructure needs. Each deployment belongs to
one tenant's dedicated database; set `VMCP_TENANT_ID` for non-default tenants.

## Consumer identity and delivery guarantees

`VMCP_AUDIT_EXPORT_CONSUMER` identifies a PostgreSQL delivery history, not a Kafka
consumer group or RabbitMQ consumer. Use a different name for each destination:
`siem-http`, `analytics-kafka`, `security-rabbitmq`, for example. All three can run
at once and each independently receives all retained events. Names accept 1–64
ASCII letters, digits, underscores and hyphens.

**Use one transport/destination per consumer name.** There is deliberately no
cross-check. Reusing a name across different destinations causes workers to
compete for the same receipts and can split the evidence between destinations.
Replicas of the same exporter may share a consumer only when publishing to the
same destination. Changing a destination preserves delivered history: use a new
consumer name when a full replay of retained events is required.

All transports deliver **at least once**. A lost acknowledgment or a crash between
broker acceptance and the PostgreSQL receipt can produce duplicates. Deduplicate
the event UUID in downstream processing. Kafka producer idempotence does not make
this database-to-broker workflow exactly once. There is no global, per-operation,
or admission-before-outcome ordering guarantee; retries and replicas can reorder
messages. Correlate using event and operation IDs.

The payload is the same JSON audit database row used by the HTTP exporter, with
its `event_id` unchanged. No tool argument/result payloads are added. Audit
metadata may still contain personal information. Follow access, privacy and
retention requirements at the destination too.

Delivery success means a 2xx HTTP response, an acknowledged Kafka produce, or a
RabbitMQ publisher confirm without a mandatory return. Broker acknowledgment
means broker acceptance, **not downstream processing or archival**. Broker
replication, retention/TTL, overflow policies and downstream acknowledgments are
operator responsibilities. A broker receipt permits database retention once all
registered consumers are delivered and other hold/operation rules allow it.

Workers publish one event at a time, using a five-second publish deadline, a
30-second fenced lease and the existing durable retries. Failures back off from
two seconds to one hour. After 20 attempts the receipt is `dead` and still blocks
retention. Broker clients reuse connections; RabbitMQ recreates its connection,
channel and confirms after a failed/uncertain publish. Shutdown stops dispatch,
bounds in-flight work, and leaves unconfirmed events retryable. See
[operations and recovery](AUDIT_OPERATIONS.md) before enabling retention.

## Build and deploy

From the repository root:

```sh
(cd core && go build -o bin/audit-exporter ./cmd/audit-exporter)
(cd core && go build -o bin/audit-exporter-kafka ./cmd/audit-exporter-kafka)
(cd core && go build -o bin/audit-exporter-rabbitmq ./cmd/audit-exporter-rabbitmq)

# Build independent images; the existing Dockerfile selects one command.
docker build -f core/Dockerfile --build-arg SERVICE=audit-exporter-kafka -t vmcp-audit-kafka:local core
docker build -f core/Dockerfile --build-arg SERVICE=audit-exporter-rabbitmq -t vmcp-audit-rabbitmq:local core
```

Run each binary/image as its own service. Supply database credentials and broker
secrets through your secret-management system. Do not put production passwords
in shell history, image layers or committed Compose files. Release artifact builds
include both new binaries; adding these commands does not publish a release.

All exporters require:

- `VMCP_AUDIT_EXPORT_DATABASE_URL`: dedicated tenant PostgreSQL URL; use
  `sslmode=verify-full` in production.
- `VMCP_AUDIT_EXPORT_CONSUMER`: independent destination identity as described above.
- `VMCP_TENANT_ID`: defaults to `default`; must match the database identity.

Provision the exporter LOGIN out of band and grant it membership in
`vmcp_audit_export` from [the role script](../deploy/postgres/audit-roles.sql),
executed once by the database owner after migrations. The export role can read
identity tables and audit events, and maintain delivery receipts. It cannot delete
audit evidence or manage legal holds. Existing installations should apply the
new `SELECT` grants on `tenant_runtime_identity` and `tenants` to their export
role; do not rerun `CREATE ROLE` statements against existing roles.

## HTTPS

Use `audit-exporter` with `VMCP_AUDIT_EXPORT_URL=https://siem.example/events`.
Each request carries `Content-Type: application/json` and
`Idempotency-Key: <event UUID>`. The sink must return 2xx only after accepting the
event durably, and must deduplicate IDs. Existing HTTPS configuration is unchanged.
The broker TLS variables below apply only to Kafka and RabbitMQ.

## Kafka

Use `audit-exporter-kafka` and configure:

- `VMCP_AUDIT_KAFKA_BROKERS`: comma-separated bootstrap `host:port` addresses,
  such as `kafka-a.example:9093,kafka-b.example:9093`.
- `VMCP_AUDIT_KAFKA_TOPIC`: pre-provisioned audit topic.
- `VMCP_AUDIT_KAFKA_SASL`: optional `PLAIN`, `SCRAM-SHA-256` or `SCRAM-SHA-512`.
- `VMCP_AUDIT_KAFKA_USERNAME` and `VMCP_AUDIT_KAFKA_PASSWORD`: required together
  when a SASL mechanism is selected. Credentials without a mechanism are rejected.

TLS with hostname verification is the default. Idempotent production and
`acks=all` are enabled. The record key is the event UUID; headers include
`event-id` and `content-type`. The producer buffer is bounded to one record and
1 MiB; oversized events fail and follow the delivery retry/dead-letter policy.
No topics, ACLs or consumer groups are created by the exporter.

Provision appropriate replication and `min.insync.replicas` for your durability
requirements; `acks=all` alone does not guarantee multiple replicas. Allow the
publisher's principal to write the topic and use idempotent production as required
by your Kafka ACL setup. Downstream applications configure their own consumer
groups, offsets and retention. SASL OAuth, Kerberos and transactions are not
implemented by this exporter.

## RabbitMQ

Use `audit-exporter-rabbitmq` and configure:

- `VMCP_AUDIT_RABBITMQ_URL`: `amqps://<user>:<password>@rabbit.example:5671/<vhost>`.
  URL-encode credentials and vhost names as needed (`%2F` for the `/` vhost).
- `VMCP_AUDIT_RABBITMQ_EXCHANGE`: pre-provisioned exchange; empty selects the
  default exchange.
- `VMCP_AUDIT_RABBITMQ_ROUTING_KEY`: required routing key; with the default
  exchange, this is the pre-provisioned queue name.

Messages use persistent delivery mode, `ContentType=application/json`,
`MessageId=<event UUID>`, and `Type=vmcp.audit.event`. Publisher confirms and
mandatory routing are always enabled. A returned/unroutable message, negative
confirm, channel failure or uncertain publish is retried; it is never counted as
delivered merely because the socket write succeeded.

Provision durable exchanges, durable queues and bindings before starting the
exporter. Prefer quorum queues when replicated durability is needed. The exporter
does not declare topology or infer durability from bindings. RabbitMQ publisher
confirms do not mean a downstream consumer processed the message. Downstream
consumers should acknowledge only after processing durably and deduplicate IDs.

## TLS, network boundaries and logs

Kafka and RabbitMQ share these optional settings:

- `VMCP_AUDIT_EXPORT_TLS_CA_FILE`: additional trusted CA certificates in PEM.
- `VMCP_AUDIT_EXPORT_TLS_CERT_FILE` and `VMCP_AUDIT_EXPORT_TLS_KEY_FILE`: paired
  PEM client certificate/private-key files for mTLS. Files are read at startup;
  restart exporters after rotating them.

TLS requires at least TLS 1.2 and server certificate/hostname verification cannot
be disabled. RabbitMQ uses URL username/password authentication; mTLS can provide
an additional transport credential, but AMQP EXTERNAL authentication is not
implemented. Kafka and RabbitMQ can use system trust when no custom CA is set.

All publisher connections enforce `VMCP_EGRESS_ALLOW_HOSTS` and
`VMCP_EGRESS_ALLOW_CIDRS`. Private, loopback and local broker addresses require
explicit permitted CIDRs; a hostname allowlist alone does not allow private IPs.
Kafka metadata advertises additional broker addresses: permit and make those
addresses reachable too, not just bootstrap hosts.

For isolated development only, `VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT=true` selects
plaintext Kafka and permits `amqp://` RabbitMQ. Never use this setting with
production credentials or evidence. It does not permit HTTP for the HTTPS exporter.

Logs use structured JSON, omit payloads/credentials and report delivery-state
counts once per minute. There is currently no exporter Prometheus endpoint. Alert
on pending/dead receipt counts, database storage growth and broker health using
restricted operational queries and your infrastructure monitoring.

## Regression tests

The isolated test stack leaves the manual benchmark stack unchanged:

```sh
docker compose -f tests/audit-exporters/compose.yaml up -d --wait --wait-timeout 180
(cd core && \
 VMCP_TEST_DATABASE_URL='postgres://vmcp:vmcp@localhost:55433/vmcp_test?sslmode=disable' \
 VMCP_TEST_KAFKA_BROKERS=localhost:19092 \
 VMCP_TEST_RABBITMQ_URL='amqp://audit:audit@localhost:5673/' \
 VMCP_REQUIRE_INTEGRATION=1 \
 go test -race -count=1 ./internal/audit ./internal/auditexport ./internal/auditkafka ./internal/auditrabbitmq)
python3 scripts/check-exporter-isolation.py
docker compose -f tests/audit-exporters/compose.yaml down -v
```

Tests exercise real PostgreSQL and brokers, HTTP delivery regression, duplicate
identities, cancellation, TLS downgrade rejection, RabbitMQ returns/channel loss
and reconnects, independent consumer receipts, retention protection and restricted
export privileges. CI runs these alongside the existing required integration suite.
The local brokers are deliberately single-node and plaintext; this is not an HA,
SASL, mTLS, broker restart or production throughput certification.
