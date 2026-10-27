package store

import (
	"context"
	"errors"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/jackc/pgx/v5"
)

// Access exists only on authenticated management requests. System workers and
// data-plane snapshot builders deliberately do not carry a management scope.
type Access struct {
	Role, Issuer, Actor string
	Groups              []string
	Write               bool
}
type accessKey struct{}

func WithAccess(ctx context.Context, a Access) context.Context {
	return context.WithValue(ctx, accessKey{}, a)
}
func AccessFrom(ctx context.Context) (Access, bool) {
	a, ok := ctx.Value(accessKey{}).(Access)
	return a, ok
}
func literal(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func ownerPredicate(ctx context.Context, column string) string {
	a, ok := AccessFrom(ctx)
	if !ok || a.Role == "admin" {
		return "TRUE"
	}
	parts := []string{"FALSE"}
	if (a.Role == "viewer" && !a.Write) || a.Role == "editor" {
		parts = append(parts, column+" IS NULL")
	}
	if len(a.Groups) > 0 {
		groups := make([]string, len(a.Groups))
		for i, g := range a.Groups {
			groups[i] = literal(g)
		}
		parts = append(parts, column+" IN (SELECT id FROM teams WHERE issuer="+literal(a.Issuer)+" AND group_value IN ("+strings.Join(groups, ",")+"))")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}
func vsPredicate(ctx context.Context, alias string) string {
	return ownerPredicate(ctx, "(SELECT owner_team_id FROM projects WHERE id="+alias+".project_id)")
}
func (t *Tx) checkOwner(ctx context.Context, team string) error {
	a, ok := AccessFrom(ctx)
	if !ok || a.Role == "admin" {
		return nil
	}
	if team == "" {
		if a.Role == "editor" || (!a.Write && a.Role == "viewer") {
			return nil
		}
		return apierror.Forbidden("default workspace requires a global role")
	}
	var allowed bool
	err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE slug=$1 AND issuer=$2 AND group_value=ANY($3::text[]))`, team, a.Issuer, nonNil(a.Groups)).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return apierror.Forbidden("resource belongs to another team")
	}
	return nil
}
func platformOnly(ctx context.Context) error {
	if a, ok := AccessFrom(ctx); ok && a.Role != "admin" {
		return apierror.Forbidden("platform admin required")
	}
	return nil
}
func (t *Tx) checkProject(ctx context.Context, slug string) error {
	var owner string
	err := t.q.QueryRow(ctx, `SELECT COALESCE(t.slug,'') FROM projects p LEFT JOIN teams t ON t.id=p.owner_team_id WHERE p.slug=$1`, slug).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.NotFound("project", slug)
	}
	if err != nil {
		return err
	}
	return t.checkOwner(ctx, owner)
}
func (t *Tx) CheckTeamLogin(ctx context.Context, issuer string, groups []string) (bool, error) {
	var ok bool
	err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE issuer=$1 AND group_value=ANY($2::text[]))`, issuer, nonNil(groups)).Scan(&ok)
	return ok, err
}

type Team struct {
	CanManage bool   `json:"canManage"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Issuer    string `json:"issuer"`
	Group     string `json:"group"`
	Version   int64  `json:"version"`
}
type Project struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	OwnerTeam string `json:"ownerTeam"`
	Version   int64  `json:"version"`
}

func (t *Tx) ListTeams(ctx context.Context) ([]Team, error) {
	rows, err := t.q.Query(ctx, `SELECT slug,name,issuer,group_value,version,`+ownerPredicate(ctx, "id")+` FROM teams ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Team{}
	for rows.Next() {
		var v Team
		if err := rows.Scan(&v.Slug, &v.Name, &v.Issuer, &v.Group, &v.Version, &v.CanManage); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (t *Tx) PutTeam(ctx context.Context, v Team) error {
	if err := platformOnly(ctx); err != nil {
		return err
	}
	if len(SlugErrors("/slug", v.Slug)) > 0 || strings.TrimSpace(v.Name) == "" || strings.TrimSpace(v.Issuer) == "" || strings.TrimSpace(v.Group) == "" {
		return apierror.Validation([]apierror.FieldError{vf("/team", "slug, name, issuer and group are required")})
	}
	t.auditTarget("team", v.Slug)
	var version int64
	err := t.q.QueryRow(ctx, `SELECT version FROM teams WHERE slug=$1`, v.Slug).Scan(&version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && version != v.Version {
		return apierror.PreconditionFailed()
	}
	_, err = t.q.Exec(ctx, `INSERT INTO teams(slug,name,issuer,group_value) VALUES($1,$2,$3,$4) ON CONFLICT(slug) DO UPDATE SET name=$2,issuer=$3,group_value=$4,version=teams.version+1`, v.Slug, v.Name, v.Issuer, v.Group)
	return mapWriteErr(err, "team", v.Slug)
}
func (t *Tx) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := t.q.Query(ctx, `SELECT p.slug,p.name,COALESCE(t.slug,''),p.version FROM projects p LEFT JOIN teams t ON t.id=p.owner_team_id WHERE `+ownerPredicate(ctx, "p.owner_team_id")+` ORDER BY p.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var v Project
		if err := rows.Scan(&v.Slug, &v.Name, &v.OwnerTeam, &v.Version); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (t *Tx) PutProject(ctx context.Context, v Project) error {
	if err := platformOnly(ctx); err != nil {
		return err
	}
	if len(SlugErrors("/slug", v.Slug)) > 0 || strings.TrimSpace(v.Name) == "" {
		return apierror.Validation([]apierror.FieldError{vf("/project", "slug and name required")})
	}
	if v.Slug == "default" && v.OwnerTeam != "" {
		return apierror.Forbidden("default project must remain unmanaged")
	}
	var teamID *string
	if v.OwnerTeam != "" {
		var id string
		if err := t.q.QueryRow(ctx, `SELECT id::text FROM teams WHERE slug=$1`, v.OwnerTeam).Scan(&id); err != nil {
			return apierror.NotFound("team", v.OwnerTeam)
		}
		teamID = &id
	}
	var version int64
	var owner string
	err := t.q.QueryRow(ctx, `SELECT p.version,COALESCE(t.slug,'') FROM projects p LEFT JOIN teams t ON t.id=p.owner_team_id WHERE p.slug=$1`, v.Slug).Scan(&version, &owner)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && version != v.Version {
		return apierror.PreconditionFailed()
	}
	if err == nil && owner != v.OwnerTeam { // ownership changes invalidate explicit cross-team grants
		if _, err = t.q.Exec(ctx, `UPDATE tool_grants SET status='revoked',version=version+1,updated_at=now() WHERE virtual_server_id IN (SELECT v.id FROM virtual_servers v JOIN projects p ON p.id=v.project_id WHERE p.slug=$1)`, v.Slug); err != nil {
			return err
		}
	}
	t.auditTarget("project", v.Slug)
	_, err = t.q.Exec(ctx, `INSERT INTO projects(slug,name,owner_team_id) VALUES($1,$2,$3::uuid) ON CONFLICT(slug) DO UPDATE SET name=$2,owner_team_id=$3::uuid,version=projects.version+1`, v.Slug, v.Name, teamID)
	return mapWriteErr(err, "project", v.Slug)
}

// credentialPredicate prevents a scoped editor from controlling a key/binding
// that also grants access outside their scope. Empty legacy grants remain default.
func credentialPredicate(ctx context.Context, kind, alias string) string {
	a, ok := AccessFrom(ctx)
	if !ok || a.Role == "admin" {
		return "TRUE"
	}
	table, col := "agent_key_servers", "agent_key_id"
	if kind == "binding" {
		table, col = "identity_binding_servers", "binding_id"
	}
	base := "SELECT 1 FROM " + table + " g JOIN virtual_servers v ON v.id=g.virtual_server_id WHERE g." + col + "=" + alias + ".id"
	pred := "NOT EXISTS (" + base + " AND NOT " + vsPredicate(ctx, "v") + ")"
	if a.Role == "team" || (a.Role == "viewer" && a.Write) {
		pred += " AND EXISTS (" + base + ")"
	}
	return "(" + pred + ")"
}
func (t *Tx) checkCredential(ctx context.Context, kind, id string) error {
	if _, ok := AccessFrom(ctx); !ok {
		return nil
	}
	table := "agent_keys"
	if kind == "binding" {
		table = "identity_bindings"
	}
	var ok bool
	err := t.q.QueryRow(ctx, `SELECT `+credentialPredicate(ctx, kind, "k")+` FROM `+table+` k WHERE k.id=$1::uuid`, id).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return apierror.Forbidden("credential grants cross team boundaries")
	}
	return nil
}

func internalAccess(ctx context.Context) context.Context {
	a, _ := AccessFrom(ctx)
	a.Role = "admin"
	return WithAccess(ctx, a)
}
func (t *Tx) PutVirtualServer(ctx context.Context, slug string, in VirtualServerInput, match *int64, create bool) (*VirtualServer, Outcome, error) {
	var oldProject string
	err := t.q.QueryRow(ctx, `SELECT p.slug FROM virtual_servers v JOIN projects p ON p.id=v.project_id WHERE v.slug=$1 AND v.tenant_id=`+tenantSQL, slug).Scan(&oldProject)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", err
	}
	project := oldProject
	if in.Project != nil {
		project = *in.Project
	}
	if project == "" {
		project = "default"
	}
	if exists {
		if err = t.checkProject(ctx, oldProject); err != nil {
			return nil, "", err
		}
		if project != oldProject {
			if err = platformOnly(ctx); err != nil {
				return nil, "", err
			}
		}
	}
	if err = t.checkProject(ctx, project); err != nil {
		return nil, "", err
	}
	// Non-tool features have no cross-team approval workflow in this release.
	for _, c := range in.Catalog {
		if err = t.checkCatalogOwner(ctx, project, c.Upstream); err != nil {
			return nil, "", err
		}
	}
	if in.Guidance != nil {
		for _, up := range in.Guidance.UpstreamInstructions {
			if err = t.checkCatalogOwner(ctx, project, up); err != nil {
				return nil, "", err
			}
		}
	}
	v, out, err := t.putVirtualServer(ctxInternal(ctx), slug, in, match, create)
	if err != nil {
		return nil, "", err
	}
	if project != oldProject {
		if _, err = t.q.Exec(ctx, `UPDATE virtual_servers SET project_id=(SELECT id FROM projects WHERE slug=$2),version=version+1 WHERE slug=$1 AND tenant_id=`+tenantSQL, slug, project); err != nil {
			return nil, "", err
		}
		if exists {
			_, err = t.q.Exec(ctx, `UPDATE tool_grants SET status='revoked',version=version+1,updated_at=now() WHERE virtual_server_id=(SELECT id FROM virtual_servers WHERE slug=$1 AND tenant_id=`+tenantSQL+`)`, slug)
			if err != nil {
				return nil, "", err
			}
		}
		if out == Unchanged {
			out = Updated
		}
	}
	v, err = t.GetVirtualServer(ctx, slug)
	return v, out, err
}
func ctxInternal(ctx context.Context) context.Context { return internalAccess(ctx) }
func (t *Tx) checkCatalogOwner(ctx context.Context, project, upstream string) error {
	var allowed bool
	err := t.q.QueryRow(ctx, `SELECT COALESCE(u.owner_team_id IS NULL OR u.owner_team_id=p.owner_team_id,false) FROM upstreams u CROSS JOIN projects p WHERE u.slug=$1 AND p.slug=$2`, upstream, project).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // Existing selection validation owns missing/orphan diagnostics.
	}
	if err != nil {
		return err
	}
	if !allowed {
		return apierror.Forbidden("resources, prompts and upstream instructions require the same owner team; cross-team approval covers tools only")
	}
	return nil
}
func (t *Tx) PutUpstream(ctx context.Context, slug string, in UpstreamInput, match *int64, create bool) (*Upstream, Outcome, error) {
	var oldOwner string
	err := t.q.QueryRow(ctx, `SELECT COALESCE(t.slug,'') FROM upstreams u LEFT JOIN teams t ON t.id=u.owner_team_id WHERE u.slug=$1 AND u.tenant_id=`+tenantSQL, slug).Scan(&oldOwner)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", err
	}
	owner := oldOwner
	if in.OwnerTeam != nil {
		owner = *in.OwnerTeam
	}
	if exists {
		if err = t.checkOwner(ctx, oldOwner); err != nil {
			return nil, "", err
		}
		if owner != oldOwner {
			if err = platformOnly(ctx); err != nil {
				return nil, "", err
			}
		}
	}
	if err = t.checkOwner(ctx, owner); err != nil {
		return nil, "", err
	}
	if owner != "" {
		var found bool
		if err = t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE slug=$1)`, owner).Scan(&found); err != nil {
			return nil, "", err
		}
		if !found {
			return nil, "", apierror.NotFound("team", owner)
		}
	}
	v, out, err := t.putUpstream(ctxInternal(ctx), slug, in, match, create)
	if err != nil {
		return nil, "", err
	}
	if owner != oldOwner {
		if _, err = t.q.Exec(ctx, `UPDATE upstreams SET owner_team_id=(SELECT id FROM teams WHERE slug=$2),version=version+1 WHERE slug=$1 AND tenant_id=`+tenantSQL, slug, owner); err != nil {
			return nil, "", err
		}
		if _, err = t.q.Exec(ctx, `UPDATE tool_grants SET status='revoked',version=version+1,updated_at=now() WHERE upstream_id=(SELECT id FROM upstreams WHERE slug=$1 AND tenant_id=`+tenantSQL+`)`, slug); err != nil {
			return nil, "", err
		}
		if out == Unchanged {
			out = Updated
		}
	}
	v, err = t.GetUpstream(ctx, slug)
	return v, out, err
}
func (t *Tx) checkVSAccess(ctx context.Context, slug string) error {
	if _, ok := AccessFrom(ctx); !ok {
		return nil
	}
	var owner string
	err := t.q.QueryRow(ctx, `SELECT COALESCE(t.slug,'') FROM virtual_servers v JOIN projects p ON p.id=v.project_id LEFT JOIN teams t ON t.id=p.owner_team_id WHERE v.slug=$1 AND v.tenant_id=`+tenantSQL, slug).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return t.checkOwner(ctx, owner)
}
func (t *Tx) checkUpstreamAccess(ctx context.Context, slug string) error {
	if _, ok := AccessFrom(ctx); !ok {
		return nil
	}
	var owner string
	err := t.q.QueryRow(ctx, `SELECT COALESCE(t.slug,'') FROM upstreams u LEFT JOIN teams t ON t.id=u.owner_team_id WHERE u.slug=$1 AND u.tenant_id=`+tenantSQL, slug).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return t.checkOwner(ctx, owner)
}

// ApplyGovernance uses the enclosing GitOps transaction/revision lock. Existing
// records that are identical are left untouched, preserving no-op revisions.
func (t *Tx) ApplyGovernance(ctx context.Context, teams []Team, projects []Project) error {
	if len(teams) == 0 && len(projects) == 0 {
		return nil
	}
	if err := platformOnly(ctx); err != nil {
		return err
	}
	existing, err := t.ListTeams(ctx)
	if err != nil {
		return err
	}
	tm := map[string]Team{}
	for _, v := range existing {
		tm[v.Slug] = v
	}
	for _, v := range teams {
		old, ok := tm[v.Slug]
		if ok && old.Name == v.Name && old.Issuer == v.Issuer && old.Group == v.Group {
			continue
		}
		v.Version = old.Version
		if err := t.PutTeam(ctx, v); err != nil {
			return err
		}
	}
	current, err := t.ListProjects(ctx)
	if err != nil {
		return err
	}
	pm := map[string]Project{}
	for _, v := range current {
		pm[v.Slug] = v
	}
	for _, v := range projects {
		old, ok := pm[v.Slug]
		if ok && old.Name == v.Name && old.OwnerTeam == v.OwnerTeam {
			continue
		}
		v.Version = old.Version
		if err := t.PutProject(ctx, v); err != nil {
			return err
		}
	}
	return nil
}
func (t *Tx) ApplyToolGrants(ctx context.Context, grants []ToolGrant) error {
	if len(grants) == 0 {
		return nil
	}
	if err := platformOnly(ctx); err != nil {
		return err
	}
	existing, err := t.ListToolGrants(ctx)
	if err != nil {
		return err
	}
	byKey := map[string]ToolGrant{}
	for _, g := range existing {
		byKey[g.VirtualServer+"\x00"+g.Upstream+"\x00"+g.Tool] = g
	}
	for _, g := range grants {
		old, ok := byKey[g.VirtualServer+"\x00"+g.Upstream+"\x00"+g.Tool]
		if ok && old.Status == g.Status && old.Reason == g.Reason {
			continue
		}
		g.Version = old.Version
		if err := t.PutToolGrant(ctx, g); err != nil {
			return err
		}
	}
	return nil
}
