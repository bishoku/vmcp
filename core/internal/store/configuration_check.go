package store

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/authz"
)

func lowerConfigurationObject(input map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range input {
		key := strings.ToLower(k)
		if _, exists := out[key]; exists {
			return nil, apierror.Validation([]apierror.FieldError{{Message: "ambiguous configuration field aliases"}})
		}
		out[key] = v
	}
	return out, nil
}
func configString(body map[string]any, key string) (string, bool) {
	v, ok := body[key]
	if !ok || v == nil {
		return "", false
	}
	s, yes := v.(string)
	return s, yes
}
func (c configurationPolicy) require(permission string, resources ...authz.Resource) error {
	for _, r := range resources {
		if !c.Policy.Decide(authz.Request{Principal: c.Who, Permission: permission, Resource: r}).Allowed {
			return apierror.Forbidden("missing configuration permission: " + permission)
		}
	}
	return nil
}
func (c configurationPolicy) any(permission string) bool {
	for _, a := range c.Policy.Assignments {
		if !a.Enabled || a.MappingID != "" || a.Principal != c.Who {
			continue
		}
		spec, ok := authz.Lookup(permission)
		if !ok || !slices.Contains(spec.Scopes, a.Scope.Kind) {
			continue
		}
		for _, r := range c.Policy.Roles {
			if r.ID == a.RoleID && slices.Contains(r.Permissions, permission) {
				return true
			}
		}
	}
	return false
}
func (t *Tx) checkConfiguration(ctx context.Context, c configurationPolicy) error {
	if c.Policy.Validate() != nil {
		return apierror.Forbidden("invalid configuration policy")
	}
	var tenantID string
	if err := t.q.QueryRow(ctx, `SELECT vmcp_tenant_id()`).Scan(&tenantID); err != nil {
		return err
	}
	if tenantID != c.Policy.Tenant {
		return apierror.Forbidden("configuration tenant mismatch")
	}

	op := c.Operation
	body, err := lowerConfigurationObject(op.Body)
	if err != nil {
		return err
	}
	op.Body = body
	c.Operation = op
	allowed := map[string]bool{}
	switch op.Family {
	case "teams":
		for _, k := range []string{"slug", "name", "issuer", "group", "version", "canmanage"} {
			allowed[k] = true
		}
	case "projects":
		for _, k := range []string{"slug", "name", "ownerteam", "version"} {
			allowed[k] = true
		}
	case "tool_grants":
		if op.Action != "bulk" {
			for _, k := range []string{"virtualserver", "upstream", "tool", "status", "reason", "version", "actor", "consumerproject", "consumerteam", "upstreamownerteam", "candecide"} {
				allowed[k] = true
			}
		}
	}
	if len(allowed) > 0 {
		for k := range body {
			if !allowed[k] {
				return apierror.Forbidden("unclassified governance field")
			}
		}
	}

	tenant := authz.Resource{Tenant: c.Policy.Tenant, Kind: "tenant"}
	if op.Family == "self" {
		return nil
	}
	if op.Family == "tool_grants" && op.Action == "bulk" {
		for _, p := range []string{"request", "approve", "reject", "revoke"} {
			if c.any("tool_grants." + p) {
				return nil
			}
		}
		return apierror.Forbidden("tool grant permission required")
	}
	if op.Family == "tool_grants" && op.Verb != "read" {
		return t.checkConfigurationGrant(ctx, c, body)
	}
	switch op.Family {
	case "audit_local":
		if op.ResourceKind != "upstream" && op.ResourceKind != "virtual_server" {
			return apierror.Forbidden("invalid local settings resource")
		}
		resource, exists, err := t.configurationResource(ctx, op.ResourceKind, op.Slug)
		if err != nil {
			return err
		}
		if !exists {
			return apierror.NotFound(op.ResourceKind, op.Slug)
		}
		permission := "audit.settings.read"
		if op.Action == "preview" {
			if err := c.require("audit.settings.read", resource); err != nil {
				return err
			}
			permission = "audit.redaction.preview"
		} else if op.Verb != "read" {
			permission = "audit.redaction.local.update"
		}
		return c.require(permission, resource)
	case "upstreams", "virtual_servers":
		return t.checkConfigurationResource(ctx, c, body)
	case "agent_keys", "identity_bindings":
		return t.checkConfigurationCredential(ctx, c, body)
	case "teams":
		if op.Verb == "read" {
			if c.any("teams.read") {
				return nil
			}
			return apierror.Forbidden("teams.read required")
		}
		if err := c.require("teams.manage", tenant); err != nil {
			return err
		}
		slug, _ := configString(body, "slug")
		var exists bool
		var issuer, group string
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE slug=$1),COALESCE((SELECT issuer FROM teams WHERE slug=$1),''),COALESCE((SELECT group_value FROM teams WHERE slug=$1),'')`, slug).Scan(&exists, &issuer, &group); err != nil {
			return err
		}
		nextIssuer, _ := configString(body, "issuer")
		nextGroup, _ := configString(body, "group")
		if !exists || nextIssuer != issuer || nextGroup != group {
			return c.require("access.mappings.manage", tenant)
		}
		return nil

	case "projects":
		if op.Verb == "read" {
			if c.any("projects.read") {
				return nil
			}
			return apierror.Forbidden("projects.read required")
		}
		slug, _ := configString(body, "slug")
		old, exists, err := t.configurationResource(ctx, "project", slug)
		if err != nil {
			return err
		}
		target := old
		target.OwnerTeam = "" // POST replaces the project owner; absent/null clears it.
		if !exists {
			target = authz.Resource{Tenant: c.Policy.Tenant, Kind: "project"}
		}
		if owner, set := configString(body, "ownerteam"); set {
			target.OwnerTeam = ""
			if owner != "" {
				team, yes, err := t.configurationResource(ctx, "team", owner)
				if err != nil {
					return err
				}
				if !yes {
					return apierror.NotFound("team", owner)
				}
				target.OwnerTeam = team.ID
			}
		}
		if err := c.require("projects.manage", target); err != nil {
			return err
		}
		if exists {
			if err := c.require("projects.manage", old); err != nil {
				return err
			}
			if old.OwnerTeam != target.OwnerTeam {
				dest := target
				dest.ID = ""
				dest.Project = ""
				return configurationTransfer(c, old, dest)
			}
		}
		return nil
	case "admission":
		return c.require("admission."+op.Action, tenant)
	case "invocation_security":
		return c.require("invocation_security."+op.Action, tenant)
	case "config":
		return t.checkConfigurationDocument(ctx, c, body)
	case "audit":
		switch op.Action {
		case "settings.read":
			return c.require("audit.settings.read", tenant)
		case "settings.update":
			fields := map[string]string{"mode": "durability.update", "retentiondays": "retention.update", "maxpayloadbytes": "retention.update", "scopes": "capture.update", "rules": "redaction.global.update", "exportpayloads": "export_policy.update", "omni": "diagnostics.update", "revision": "", "virtualservers": "redaction.local.update", "upstreams": "redaction.local.update"}
			// Full-document settings remain tenant scoped. Local settings require their
			// separate resource endpoints; a global map is never a scoped write bypass.
			if err := c.require("audit.settings.read", tenant); err != nil {
				return err
			}
			for k := range body {
				if _, ok := fields[k]; !ok {
					return apierror.Forbidden("unclassified audit setting")
				}
			}
			for _, p := range fields {
				if p != "" {
					if err := c.require("audit."+p, tenant); err != nil {
						return err
					}
				}
			}
			return nil
		case "redaction.preview":
			// Preview accepts a full settings document: conservative tenant gate until
			// source-owned local settings APIs can supply a bounded effective view.
			if err := c.require("audit.settings.read", tenant); err != nil {
				return err
			}
			return c.require("audit.redaction.preview", tenant)
		}
	case "control_tokens":
		if err := c.require(configurationPermission(op), tenant); err != nil {
			return err
		}
		if op.Verb == "create" {
			if err := c.require("control_tokens.access.update", tenant); err != nil {
				return err
			}
		}
		for k := range body {
			p := ""
			switch k {
			case "scope":
				p = "access.update"
			case "expiresat":
				p = "constraints.update"
			case "slug", "description":
			default:
				return apierror.Forbidden("unclassified control-token field")
			}
			if p != "" {
				if err := c.require("control_tokens."+p, tenant); err != nil {
					return err
				}
			}
		}
		if !op.GlobalAdmin {
			scope, _ := configString(body, "scope")
			if scope == "admin" {
				return apierror.Forbidden("issuing platform admin credentials requires a global administrator")
			}
			if op.Action == "rotate" {
				var assigned bool
				if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_assignments a JOIN admin_tokens t ON t.id::text=a.principal_id WHERE a.principal_kind='control_token' AND a.enabled AND t.slug=$1 AND t.tenant_id=`+tenantSQL+`)`, op.Slug).Scan(&assigned); err != nil {
					return err
				}
				if assigned {
					var self bool
					if c.Who.Kind == "control_token" {
						if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_tokens WHERE id::text=$1 AND slug=$2 AND tenant_id=`+tenantSQL+`)`, c.Who.ID, op.Slug).Scan(&self); err != nil {
							return err
						}
					}
					if !self {
						return apierror.Forbidden("rotating an assigned control identity requires a global administrator")
					}
				}
			}
			if op.Action == "rotate" {
				var privileged bool
				if err := t.q.QueryRow(ctx, `SELECT scope='admin' FROM admin_tokens WHERE slug=$1 AND tenant_id=`+tenantSQL, op.Slug).Scan(&privileged); err != nil {
					return err
				}
				if privileged {
					return apierror.Forbidden("rotating platform admin credentials requires a global administrator")
				}
			}
		}
		return nil
	}
	permission := configurationPermission(op)
	if strings.HasSuffix(permission, ".read") && (op.Family == "tool_catalog" || op.Family == "tool_grants") {
		if c.any(permission) {
			return nil
		}
		return apierror.Forbidden("missing configuration permission: " + permission)
	}
	return c.require(permission, tenant)
}
func configurationTransfer(c configurationPolicy, old, target authz.Resource) error {
	if err := c.require("ownership.transfer", old, target); err != nil {
		return err
	}
	if old.OwnerTeam == "" || target.OwnerTeam == "" {
		return c.require("ownership.transfer", authz.Resource{Tenant: c.Policy.Tenant, Kind: "tenant"})
	}
	return nil
}
func (t *Tx) checkConfigurationResource(ctx context.Context, c configurationPolicy, body map[string]any) error {
	op := c.Operation
	kind := "upstream"
	if op.Family == "virtual_servers" {
		kind = "virtual_server"
	}
	if op.Slug == "" && op.Verb == "read" {
		if c.any(configurationPermission(op)) {
			return nil
		}
		return apierror.Forbidden("missing configuration permission: " + configurationPermission(op))
	}
	slug := op.Slug
	if slug == "" {
		slug, _ = configString(body, "slug")
	}
	old, exists, err := t.configurationResource(ctx, kind, slug)
	if err != nil {
		return err
	}
	if op.Verb == "read" || op.Verb == "delete" || op.Action != "" {
		if !exists {
			return apierror.NotFound(kind, slug)
		}
		return c.require(configurationPermission(op), old)
	}
	if !exists && op.Verb == "update" {
		return apierror.NotFound(kind, slug)
	}
	target := old
	if !exists {
		target = authz.Resource{Tenant: c.Policy.Tenant, Kind: kind}
	}
	if kind == "upstream" {
		if owner, set := configString(body, "ownerteam"); set {
			target.OwnerTeam = ""
			if owner != "" {
				team, ok, err := t.configurationResource(ctx, "team", owner)
				if err != nil {
					return err
				}
				if !ok {
					return apierror.NotFound("team", owner)
				}
				target.OwnerTeam = team.ID
			}
		}
	} else {
		project, set := configString(body, "project")
		if !exists && !set {
			project = "default"
			set = true
		}
		if set {
			if project == "" {
				project = "default"
			}
			p, ok, err := t.configurationResource(ctx, "project", project)
			if err != nil {
				return err
			}
			if !ok {
				return apierror.NotFound("project", project)
			}
			target.Project = p.ID
			target.OwnerTeam = p.OwnerTeam
		}
	}
	verb := "update"
	if !exists || op.Verb == "create" {
		verb = "create"
	}
	domains := []authz.Resource{old}
	if !exists {
		domains = []authz.Resource{target}
	}
	if err := c.require(op.Family+"."+verb, domains...); err != nil {
		return err
	}
	if exists && (old.OwnerTeam != target.OwnerTeam || old.Project != target.Project) {
		dest := target
		dest.ID = ""
		dest.Epoch = ""
		if err := configurationTransfer(c, old, dest); err != nil {
			return err
		}
	}
	fields := map[string]string{}
	if kind == "upstream" {
		fields = map[string]string{"slug": "", "name": "", "description": "", "ownerteam": "", "url": "connection.update", "fallbackurls": "connection.update", "kind": "connection.update", "transport": "connection.update", "enabled": "activation.update", "headers": "credentials.update", "auth": "credentials.update", "tls": "tls.update", "requestheadermappings": "identity_forwarding.update", "callercontext": "identity_forwarding.update", "rest": "rest.manage"}
	} else {
		fields = map[string]string{"slug": "", "name": "", "description": "", "guidance": "", "tools": "", "catalog": "", "project": "", "enabled": "publication.update", "omnienabled": "publication.update", "authrequired": "policy.update", "pii": "policy.update", "policy": "policy.update", "ratelimitperminute": "policy.update", "clientfeatures": "client_features.update"}
	}
	for k := range body {
		permission, ok := fields[k]
		if !ok {
			return apierror.Forbidden("unclassified configuration field: " + k)
		}
		if permission != "" {
			if err := c.require(op.Family+"."+permission, domains...); err != nil {
				return err
			}
		}
	}
	// PUT defaults may change protected values even when omitted. Require the
	// complete protected-field set, rather than treating an omission as harmless.
	if op.Verb == "replace" {
		for _, permission := range fields {
			if permission != "" {
				if err := c.require(op.Family+"."+permission, domains...); err != nil {
					return err
				}
			}
		}
	}
	if kind == "upstream" {
		if tls, ok := body["tls"].(map[string]any); ok {
			for k := range tls {
				if strings.EqualFold(k, "clientKeyPem") || strings.EqualFold(k, "clientCertPem") {
					if err := c.require("upstreams.credentials.update", domains...); err != nil {
						return err
					}
				}
			}
		}
		if _, ok := body["rest"]; ok {
			if err := c.require("upstreams.credentials.update", domains...); err != nil {
				return err
			}
		}
		if rest, ok := body["rest"].(map[string]any); ok {
			for k, v := range rest {
				if strings.EqualFold(k, "mock") && v != nil {
					if err := c.require("upstreams.mock.sync", domains...); err != nil {
						return err
					}
				}
			}
		}

	}
	return nil
}
func (t *Tx) checkConfigurationCredential(ctx context.Context, c configurationPolicy, body map[string]any) error {
	op := c.Operation
	permission := configurationPermission(op)
	if op.Slug == "" && (op.Verb == "read" || op.Action == "evaluate") {
		if c.any(permission) {
			return nil
		}
		return apierror.Forbidden("missing credential permission")
	}
	table, link, col := "agent_keys", "agent_key_servers", "agent_key_id"
	if op.Family == "identity_bindings" {
		table, link, col = "identity_bindings", "identity_binding_servers", "binding_id"
	}
	domains := []authz.Resource{}
	if op.Slug != "" {
		rows, err := t.q.Query(ctx, `SELECT v.slug FROM `+table+` k JOIN `+link+` g ON g.`+col+`=k.id JOIN virtual_servers v ON v.id=g.virtual_server_id WHERE k.slug=$1 AND k.tenant_id=`+tenantSQL, op.Slug)
		if err != nil {
			return err
		}
		slugs := []string{}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				rows.Close()
				return err
			}
			slugs = append(slugs, s)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, s := range slugs {
			r, _, err := t.configurationResource(ctx, "virtual_server", s)
			if err != nil {
				return err
			}
			domains = append(domains, r)
		}
	}
	if op.Slug != "" && len(domains) == 0 {
		var exists bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE slug=$1 AND tenant_id=`+tenantSQL+`)`, op.Slug).Scan(&exists); err != nil {
			return err
		}
		if exists {
			domains = append(domains, authz.Resource{Tenant: c.Policy.Tenant, Kind: "tenant"})
		}
	}
	if raw, ok := body["virtualservers"]; ok {
		list, yes := raw.([]any)
		if !yes && raw != nil {
			return apierror.Validation([]apierror.FieldError{{Message: "virtualServers must be an array"}})
		}
		for _, value := range list {
			s, ok := value.(string)
			if !ok {
				return apierror.Validation([]apierror.FieldError{{Message: "virtualServers must contain strings"}})
			}
			r, exists, err := t.configurationResource(ctx, "virtual_server", s)
			if err != nil {
				return err
			}
			if !exists {
				return apierror.NotFound("virtual server", s)
			}
			domains = append(domains, r)
		}
		if len(list) == 0 {
			domains = append(domains, authz.Resource{Tenant: c.Policy.Tenant, Kind: "tenant"})
		}
	}
	if op.Verb == "replace" {
		raw, provided := body["virtualservers"]
		if !provided || raw == nil {
			domains = append(domains, authz.Resource{Tenant: c.Policy.Tenant, Kind: "tenant"})
		}
	}
	if len(domains) == 0 {
		domains = []authz.Resource{{Tenant: c.Policy.Tenant, Kind: "tenant"}}
	}
	verb := op.Verb
	if verb == "replace" {
		var exists bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE slug=$1 AND tenant_id=`+tenantSQL+`)`, op.Slug).Scan(&exists); err != nil {
			return err
		}
		verb = "create"
		if exists {
			verb = "update"
		}
		permission = op.Family + "." + verb
	}
	if err := c.require(permission, domains...); err != nil {
		return err
	}
	if verb == "create" {
		if err := c.require(op.Family+".access.update", domains...); err != nil {
			return err
		}
	}
	for k := range body {
		p := ""
		switch k {
		case "slug", "description":
		case "virtualservers":
			p = "access.update"
		case "conditions":
			p = "rules.update"
		case "ipallowlist", "expiresat", "ratelimitperminute", "enabled":
			p = "constraints.update"
		case "auditcorrelation":
			p = "audit_correlation.update"
		default:
			return apierror.Forbidden("unclassified credential field")
		}
		if p != "" {
			if err := c.require(op.Family+"."+p, domains...); err != nil {
				return err
			}
		}
	}
	if op.Verb == "replace" {
		for _, p := range []string{"rules.update", "access.update", "constraints.update"} {
			if err := c.require(op.Family+"."+p, domains...); err != nil {
				return err
			}
		}
	}
	return nil
}
func (t *Tx) checkConfigurationGrant(ctx context.Context, c configurationPolicy, body map[string]any) error {
	s, _ := configString(body, "status")
	kind, slug, permission := "upstream", "", ""
	switch s {
	case "pending":
		kind = "virtual_server"
		slug, _ = configString(body, "virtualserver")
		permission = "request"
	case "approved":
		permission = "approve"
	case "rejected":
		permission = "reject"
	case "revoked":
		permission = "revoke"
	default:
		return apierror.Validation([]apierror.FieldError{{Message: "unknown grant status"}})
	}
	if kind == "upstream" {
		slug, _ = configString(body, "upstream")
	}
	r, ok, err := t.configurationResource(ctx, kind, slug)
	if err != nil {
		return err
	}
	if !ok {
		return apierror.NotFound(kind, slug)
	}
	return c.require("tool_grants."+permission, r)
}
func (t *Tx) checkConfigurationDocument(ctx context.Context, c configurationPolicy, body map[string]any) error {
	tenant := authz.Resource{Tenant: c.Policy.Tenant, Kind: "tenant"}
	op := c.Operation
	if err := c.require(configurationPermission(op), tenant); err != nil {
		return err
	}
	if op.Action == "export" {
		var configured bool
		if err := t.q.QueryRow(ctx, `SELECT COALESCE(octet_length(ciphertext)>0,false) FROM invocation_security WHERE singleton`).Scan(&configured); err != nil {
			return err
		}
		if configured {
			if err := c.require("invocation_security.read", tenant); err != nil {
				return err
			}
		}
		for _, p := range []string{"upstreams.read", "virtual_servers.read", "identity_bindings.read", "teams.read", "projects.read", "tool_grants.read", "audit.settings.read"} {
			if err := c.require(p, tenant); err != nil {
				return err
			}
		}
		return nil
	}
	if op.DryRun {
		if err := c.require("config.validate", tenant); err != nil {
			return err
		}
	}
	families := map[string]string{"upstreams": "upstreams", "virtualservers": "virtual_servers", "bindings": "identity_bindings", "teams": "teams", "projects": "projects", "toolgrants": "tool_grants"}
	for key, family := range families {
		values, _ := body[key].([]any)
		for _, raw := range values {
			item, ok := raw.(map[string]any)
			if !ok {
				return apierror.Validation([]apierror.FieldError{{Message: "configuration entries must be objects"}})
			}
			item, err := lowerConfigurationObject(item)
			if err != nil {
				return err
			}
			slug, _ := configString(item, "slug")
			nested := c
			nested.Operation = ConfigurationOperation{Family: family, Verb: "replace", Slug: slug, Body: item, GlobalAdmin: op.GlobalAdmin}
			if err := t.checkConfiguration(ctx, nested); err != nil {
				return err
			}
			if op.Refresh && family == "upstreams" {
				r, exists, err := t.configurationResource(ctx, "upstream", slug)
				if err != nil {
					return err
				}
				if !exists {
					r = authz.Resource{Tenant: c.Policy.Tenant, Kind: "upstream"}
					if team, set := configString(item, "ownerteam"); set && team != "" {
						owner, _, err := t.configurationResource(ctx, "team", team)
						if err != nil {
							return err
						}
						r.OwnerTeam = owner.ID
					}
				}
				if err := c.require("upstreams.catalog.refresh", r); err != nil {
					return err
				}
			}
		}
	}
	if _, ok := body["invocationsecurity"].(map[string]any); ok {
		if err := c.require("invocation_security.manage", tenant); err != nil {
			return err
		}
	}
	if raw, ok := body["auditsettings"].(map[string]any); ok {
		nested := c
		nested.Operation = ConfigurationOperation{Family: "audit", Action: "settings.update", Body: raw}
		if err := t.checkConfiguration(ctx, nested); err != nil {
			return err
		}
	}
	if op.Prune {
		if err := c.require("config.prune", tenant); err != nil {
			return err
		}
		// Prune is tenant GitOps; require tenant deletion rights for every managed
		// collection before inspecting omitted items or fetching remote catalogs.
		for _, p := range []string{"upstreams.delete", "virtual_servers.delete"} {
			if err := c.require(p, tenant); err != nil {
				return err
			}
		}
		if _, ok := body["bindings"]; ok {
			if err := c.require("identity_bindings.delete", tenant); err != nil {
				return err
			}
		}
	}
	for key := range body {
		if key != "apiversion" && key != "auditsettings" && key != "invocationsecurity" {
			if _, ok := families[key]; !ok {
				return apierror.Forbidden(fmt.Sprintf("unclassified configuration document field: %s", key))
			}
		}
	}
	return nil
}
