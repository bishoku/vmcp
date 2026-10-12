# Dedicated tenant operations

## Tenant isolation model

vMCP hosts multiple tenants using **dedicated deployments and databases**, routed
by exact public hostnames. It does not offer shared-schema tenant switching.
Each tenant has its own control/data plane processes, PostgreSQL database and
LOGIN identities, encryption/cookie keys, OIDC client/audience/bindings, egress
grants and audit exporter identity. This deliberately makes missing row predicates incapable of selecting another tenant's database. Shared-schema
RLS is unnecessary for this model and is not claimed as implemented.

Set `VMCP_TENANT_ID` consistently on every plane and operational command. Run
`cmd/migrate` with that ID before launching a new dedicated tenant. A database
identity is bound only while empty; it cannot be reassigned by changing the
runtime environment. Startup rejects a different ID or multiple tenant rows.
Existing deployments retain ID `default` and their internal row relationships.
The internal configuration tenant row remains `default`; the authoritative hosted
tenant boundary is its immutable database binding, not a request-selected SQL
predicate. API `/info`, Settings and exported audit rows identify the hosted tenant.

`cmd/tenant-router` consumes an operator-owned `VMCP_TENANT_DIRECTORY` JSON array
of `{id,host,control,data,ui}`. All target origins require HTTPS; tenant plane
origins cannot be shared. Its HTTPS listener uses `VMCP_TENANT_CERT/KEY`.
Unknown hosts fail, forwarded tenant headers are replaced, and each tenant plane
still authenticates credentials independently. DNS/TLS certificates and ingress
must cover exactly the approved public hosts. Tenant onboarding is an operator
workflow; there is no API to register arbitrary routing destinations. Egress
controls apply to the router. Prefer an independently operated ingress with the
same exact-host routing if it already provides this functionality.

Deploy one Helm release/namespace per tenant and apply separate network policies.
Revoke PUBLIC database CONNECT privileges and grant each tenant LOGIN access only
to its own database; never reuse migration-owner credentials as runtime identities.
Use distinct master/recovery keys even though new ciphertext also authenticates
the configured tenant context. Tenant IDs cannot be renamed after storing data
without an explicitly designed re-encryption migration. Legacy unversioned
ciphertext lacks tenant context and must be rotated before sensitive workloads.
Cookie signing and Redis bucket prefixes include tenant identity. Per-process
caches, session stores, snapshots and egress grants never serve another tenant.
Redis DBs/ACLs and exporter destinations must also be isolated where operators
or tenants can directly access those services. Limit resource quotas per tenant;
the router does not allocate shared process/database capacity between tenants.

No deployment can protect against an operator deliberately giving tenants the
same keys, database privileges or identity-provider audiences. Infrastructure
ACLs, independent tenant identities and backup/export access reviews are required
release gates. Restore into the original tenant identity, retain its keys and
verify the database binding before exposing the service. Direct plane endpoints
must remain private to the approved ingress/router.
