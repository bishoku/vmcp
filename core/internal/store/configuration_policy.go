package store

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConfigurationOperation struct {
	ResourceKind           string
	Family                 string
	Verb                   string
	Slug                   string
	Action                 string
	Body                   map[string]any
	Prune, DryRun, Refresh bool
	GlobalAdmin            bool
}
type configurationPolicy struct {
	Policy    authz.Policy
	Who       authz.Principal
	Operation ConfigurationOperation
}
type configurationKey struct{}
type configurationInternalKey struct{}

func WithConfigurationPolicy(ctx context.Context, p authz.Policy, who authz.Principal, op ConfigurationOperation) context.Context {
	return context.WithValue(ctx, configurationKey{}, configurationPolicy{p, who, op})
}
func HasConfigurationPolicy(ctx context.Context) bool {
	_, ok := ctx.Value(configurationKey{}).(configurationPolicy)
	return ok
}
func configurationFrom(ctx context.Context) (configurationPolicy, bool) {
	v, ok := ctx.Value(configurationKey{}).(configurationPolicy)
	return v, ok
}
func configurationPermission(op ConfigurationOperation) string {
	if op.Action != "" {
		return op.Family + "." + op.Action
	}
	return op.Family + "." + op.Verb
}
func configurationInternal(ctx context.Context) context.Context {
	return context.WithValue(ctx, configurationInternalKey{}, true)
}
func configurationSQL(ctx context.Context, kind, alias string) string {
	v, ok := configurationFrom(ctx)
	if !ok {
		return "TRUE"
	}
	if v.Operation.Verb != "read" && v.Operation.Action == "" {
		return "TRUE"
	}
	if internal, _ := ctx.Value(configurationInternalKey{}).(bool); internal {
		return "TRUE"
	}
	return configurationPermissionSQL(v.Policy, v.Who, configurationPermission(v.Operation), kind, alias)
}
func configurationPermissionSQL(p authz.Policy, who authz.Principal, permission, kind, alias string) string {
	if p.Validate() != nil {
		return "FALSE"
	}
	spec, ok := authz.Lookup(permission)
	if !ok {
		return "FALSE"
	}
	// All column names are selected by code; only policy values use SQL quoting.
	id, owner, project, epoch := "''", "NULL", "NULL", "''"
	switch kind {
	case "upstream":
		if alias != "u" {
			return "FALSE"
		}
		id = "u.id::text"
		owner = "u.owner_team_id"
		epoch = "COALESCE((SELECT epoch_id::text FROM audit_resource_epochs ce WHERE ce.resource_kind='upstream' AND ce.resource_id=u.id AND ce.end_revision IS NULL AND ce.tenant_id=vmcp_tenant_id()),'')"
	case "virtual_server":
		if alias != "v" {
			return "FALSE"
		}
		id = "v.id::text"
		owner = "(SELECT owner_team_id FROM projects WHERE id=v.project_id)"
		project = "v.project_id"
		epoch = "COALESCE((SELECT epoch_id::text FROM audit_resource_epochs ce WHERE ce.resource_kind='virtual_server' AND ce.resource_id=v.id AND ce.end_revision IS NULL AND ce.tenant_id=vmcp_tenant_id()),'')"
	case "team":
		if alias != "teams" {
			return "FALSE"
		}
		id = "teams.id::text"
		owner = "teams.id"
	case "project":
		if alias != "p" {
			return "FALSE"
		}
		id = "p.id::text"
		owner = "p.owner_team_id"
		project = "p.id"
	default:
		return "FALSE"
	}
	parts := []string{"FALSE"}
	for _, a := range p.Assignments {
		if !a.Enabled || a.MappingID != "" || a.Principal != who || !slices.Contains(spec.Scopes, a.Scope.Kind) {
			continue
		}
		granted := false
		for _, r := range p.Roles {
			if r.ID == a.RoleID && slices.Contains(r.Permissions, permission) {
				granted = true
				break
			}
		}
		if !granted {
			continue
		}
		match := "FALSE"
		switch a.Scope.Kind {
		case "tenant":
			match = "TRUE"
		case "team":
			match = owner + "::text=" + literal(a.Scope.ID)
		case "project":
			match = project + "::text=" + literal(a.Scope.ID)
		case "unmanaged":
			if kind == "upstream" || kind == "virtual_server" {
				match = owner + " IS NULL"
			}
		case kind:
			match = id + "=" + literal(a.Scope.ID)
		}
		if a.Scope.Epoch != "" {
			match = "(" + match + ") AND " + epoch + "=" + literal(a.Scope.Epoch)
		}
		parts = append(parts, "COALESCE(("+match+"),FALSE)")
	}
	return "(vmcp_tenant_id()=" + literal(p.Tenant) + " AND (" + strings.Join(parts, " OR ") + "))"
}
func configurationCredentialSQL(ctx context.Context, kind, alias string) string {
	v, ok := configurationFrom(ctx)
	if !ok {
		return "TRUE"
	}
	if v.Operation.Verb != "read" && v.Operation.Action == "" {
		return "TRUE"
	}
	if internal, _ := ctx.Value(configurationInternalKey{}).(bool); internal {
		return "TRUE"
	}
	table, col := "agent_key_servers", "agent_key_id"
	if kind == "binding" {
		table, col = "identity_binding_servers", "binding_id"
	}
	base := "SELECT 1 FROM " + table + " g JOIN virtual_servers v ON v.id=g.virtual_server_id WHERE g." + col + "=" + alias + ".id"
	predicate := configurationPermissionSQL(v.Policy, v.Who, configurationPermission(v.Operation), "virtual_server", "v")
	tenant := v.Policy.Decide(authz.Request{Principal: v.Who, Permission: configurationPermission(v.Operation), Resource: authz.Resource{Tenant: v.Policy.Tenant, Kind: "tenant"}}).Allowed
	empty := "FALSE"
	if tenant {
		empty = "TRUE"
	}
	return "(vmcp_tenant_id()=" + literal(v.Policy.Tenant) + " AND NOT EXISTS (" + base + " AND NOT " + predicate + ") AND (EXISTS (" + base + ") OR " + empty + "))"
}
func (t *Tx) configurationResource(ctx context.Context, kind, slug string) (authz.Resource, bool, error) {
	var r authz.Resource
	r.Kind = kind
	var query string
	switch kind {
	case "upstream":
		query = `SELECT vmcp_tenant_id(),u.id::text,'',COALESCE(u.owner_team_id::text,''),COALESCE(e.epoch_id::text,'') FROM upstreams u LEFT JOIN audit_resource_epochs e ON e.resource_kind='upstream' AND e.resource_id=u.id AND e.end_revision IS NULL AND e.tenant_id=vmcp_tenant_id() WHERE u.slug=$1 AND u.tenant_id=` + tenantSQL
	case "virtual_server":
		query = `SELECT vmcp_tenant_id(),v.id::text,v.project_id::text,COALESCE(p.owner_team_id::text,''),COALESCE(e.epoch_id::text,'') FROM virtual_servers v JOIN projects p ON p.id=v.project_id LEFT JOIN audit_resource_epochs e ON e.resource_kind='virtual_server' AND e.resource_id=v.id AND e.end_revision IS NULL AND e.tenant_id=vmcp_tenant_id() WHERE v.slug=$1 AND v.tenant_id=` + tenantSQL
	case "project":
		query = `SELECT vmcp_tenant_id(),id::text,id::text,COALESCE(owner_team_id::text,''),'' FROM projects WHERE slug=$1`
	case "team":
		query = `SELECT vmcp_tenant_id(),id::text,'',id::text,'' FROM teams WHERE slug=$1`
	default:
		return r, false, apierror.Forbidden("unsupported configuration resource")
	}
	err := t.q.QueryRow(ctx, query, slug).Scan(&r.Tenant, &r.ID, &r.Project, &r.OwnerTeam, &r.Epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}
func (t *Tx) ConfigurationResource(ctx context.Context, kind, slug string) (authz.Resource, bool, error) {
	return t.configurationResource(ctx, kind, slug)
}
func (t *Tx) CheckConfiguration(ctx context.Context, p authz.Policy, who authz.Principal, op ConfigurationOperation) error {
	return t.checkConfiguration(ctx, configurationPolicy{p, who, op})
}
func (t *Tx) checkConfigurationRevision(ctx context.Context) error {
	v, ok := configurationFrom(ctx)
	if !ok {
		return nil
	}
	var revision int64
	if err := t.q.QueryRow(ctx, `SELECT revision FROM authorization_state WHERE singleton FOR SHARE`).Scan(&revision); err != nil {
		return err
	}
	if revision != v.Policy.Revision {
		return apierror.PreconditionFailed()
	}
	return t.checkConfiguration(ctx, v)
}

type configurationReadKey struct{}
type contextDB struct{ pool *pgxpool.Pool }

func (d contextDB) query(ctx context.Context) dbtx {
	if q, ok := ctx.Value(configurationReadKey{}).(pgx.Tx); ok {
		return q
	}
	return d.pool
}
func (d contextDB) Exec(ctx context.Context, q string, a ...any) (pgconn.CommandTag, error) {
	return d.query(ctx).Exec(ctx, q, a...)
}
func (d contextDB) Query(ctx context.Context, q string, a ...any) (pgx.Rows, error) {
	return d.query(ctx).Query(ctx, q, a...)
}
func (d contextDB) QueryRow(ctx context.Context, q string, a ...any) pgx.Row {
	return d.query(ctx).QueryRow(ctx, q, a...)
}
func (s *Store) ConfigurationRead(ctx context.Context, fn func(context.Context) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	t := &Tx{q: tx}
	// The audit producer pinned a domain before the read; bind it to this snapshot.
	if e, ok := audit.Transaction(ctx); ok {
		current, err := controlAuditCurrent(ctx, tx, e.Access)
		if err != nil {
			return err
		}
		if !current {
			return apierror.PreconditionFailed()
		}
	}
	v, _ := configurationFrom(ctx)
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM authorization_state WHERE singleton`).Scan(&revision); err != nil {
		return err
	}
	if revision != v.Policy.Revision {
		return apierror.PreconditionFailed()
	}
	if err = t.checkConfiguration(ctx, v); err != nil {
		return err
	}
	if err = fn(context.WithValue(ctx, configurationReadKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func upstreamConfigurationPredicate(ctx context.Context) string {
	if HasConfigurationPolicy(ctx) {
		return configurationSQL(ctx, "upstream", "u")
	}
	return ownerPredicate(ctx, "u.owner_team_id")
}
func projectConfigurationPredicate(ctx context.Context) string {
	if HasConfigurationPolicy(ctx) {
		return configurationSQL(ctx, "project", "p")
	}
	return ownerPredicate(ctx, "p.owner_team_id")
}

func configurationGrantDecisionSQL(ctx context.Context) string {
	if c, ok := configurationFrom(ctx); ok {
		return "(" + configurationPermissionSQL(c.Policy, c.Who, "tool_grants.approve", "upstream", "u") + " OR " + configurationPermissionSQL(c.Policy, c.Who, "tool_grants.reject", "upstream", "u") + " OR " + configurationPermissionSQL(c.Policy, c.Who, "tool_grants.revoke", "upstream", "u") + ")"
	}
	return ownerPredicate(ctx, "u.owner_team_id")
}
func ConfigurationAllowsTenant(ctx context.Context, permission string) bool {
	c, ok := configurationFrom(ctx)
	return ok && c.require(permission, authz.Resource{Tenant: c.Policy.Tenant, Kind: "tenant"}) == nil
}

// ConfigurationPermission exposes only a decision for the authenticated caller.
// Resource facts are resolved here, never supplied as IDs or ownership claims.
func (t *Tx) ConfigurationPermission(ctx context.Context, p authz.Policy, who authz.Principal, op ConfigurationOperation, permission string) error {
	c := configurationPolicy{Policy: p, Who: who, Operation: op}
	if err := t.checkConfiguration(ctx, configurationPolicy{Policy: p, Who: who, Operation: ConfigurationOperation{Family: "self"}}); err != nil {
		return err
	}
	if permission == "ownership.transfer" && op.Family != "upstreams" && op.Family != "virtual_servers" && op.Family != "projects" {
		return apierror.Forbidden("unrelated ownership permission")
	}
	if _, ok := authz.Lookup(permission); !ok || (permission != "ownership.transfer" && !strings.HasPrefix(permission, op.Family+".")) {
		return apierror.Forbidden("unrelated permission")
	}
	// Collection affordances reveal no resource names and grant no operation.
	if op.Slug == "" && op.Body == nil && permission != "ownership.transfer" {
		if c.any(permission) {
			return nil
		}
		return apierror.Forbidden("permission unavailable")
	}
	if op.Family == "agent_keys" || op.Family == "identity_bindings" || op.Family == "control_tokens" {
		if permission == "ownership.transfer" {
			return apierror.Forbidden("unsupported scope")
		}
		c.Operation.Action = strings.TrimPrefix(permission, op.Family+".")
		c.Operation.Verb = "action"
		return t.checkConfiguration(ctx, c)
	}
	kind := map[string]string{"upstreams": "upstream", "virtual_servers": "virtual_server", "teams": "team", "projects": "project"}[op.Family]
	if kind == "" {
		return c.require(permission, authz.Resource{Tenant: p.Tenant, Kind: "tenant"})
	}
	resource, exists, err := t.configurationResource(ctx, kind, op.Slug)
	if err != nil {
		return err
	}
	if op.Slug != "" && !exists {
		return apierror.Forbidden("resource unavailable")
	}
	if !exists {
		resource = authz.Resource{Tenant: p.Tenant, Kind: kind}
		body, err := lowerConfigurationObject(op.Body)
		if err != nil {
			return err
		}
		if kind == "upstream" {
			if owner, _ := configString(body, "ownerteam"); owner != "" {
				team, found, e := t.configurationResource(ctx, "team", owner)
				if e != nil {
					return e
				}
				if !found {
					return apierror.Forbidden("target unavailable")
				}
				resource.OwnerTeam = team.ID
			}
		} else if kind == "virtual_server" {
			project, _ := configString(body, "project")
			if project == "" {
				project = "default"
			}
			projectResource, found, e := t.configurationResource(ctx, "project", project)
			if e != nil {
				return e
			}
			if !found {
				return apierror.Forbidden("target unavailable")
			}
			resource.Project = projectResource.ID
			resource.OwnerTeam = projectResource.OwnerTeam
		}
	}
	return c.require(permission, resource)
}
