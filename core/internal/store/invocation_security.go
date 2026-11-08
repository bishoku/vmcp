package store

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/invocation"
	"github.com/bishoku/vmcp/core/internal/secrets"
)

func loadInvocationSecurity(ctx context.Context, q dbtx, box *secrets.Box) (invocation.Settings, error) {
	s := invocation.Settings{Services: map[string]invocation.Service{}, Upstreams: map[string]invocation.UpstreamPolicy{}}
	var cipher []byte
	if err := q.QueryRow(ctx, `SELECT version,ciphertext FROM invocation_security WHERE singleton`).Scan(&s.Revision, &cipher); err != nil {
		return s, err
	}
	if len(cipher) > 0 {
		raw, err := box.OpenBound(cipher, "invocation-security", "configuration")
		if err != nil {
			return s, err
		}
		rev := s.Revision
		if err = json.Unmarshal(raw, &s); err != nil {
			return s, err
		}
		s.Revision = rev
	}
	if s.Services == nil {
		s.Services = map[string]invocation.Service{}
	}
	if s.Upstreams == nil {
		s.Upstreams = map[string]invocation.UpstreamPolicy{}
	}
	return s, s.Validate()
}
func (t *Tx) InvocationSecurity(ctx context.Context) (invocation.Settings, error) {
	return loadInvocationSecurity(ctx, t.q, t.box)
}
func (t *Tx) SaveInvocationSecurity(ctx context.Context, s invocation.Settings) error {
	old, err := t.InvocationSecurity(ctx)
	if err != nil {
		return err
	}
	if old.Revision != s.Revision {
		return apierror.PreconditionFailed()
	}
	if err := retainInvocationCredentials(s.Services, old.Services); err != nil {
		return err
	}
	for id, policy := range s.Upstreams {
		if err := retainInvocationCredentials(policy.Services, old.Upstreams[id].Services); err != nil {
			return err
		}
	}
	if err = s.Validate(); err != nil {
		return apierror.New(422, "invalid_invocation_security", "Invalid invocation security", err.Error())
	}
	changed := make([]string, 0)
	for id, policy := range s.Upstreams {
		if previous, exists := old.Upstreams[id]; exists && reflect.DeepEqual(previous, policy) {
			continue
		}
		if err := t.validateInvocationContext(ctx, id, policy); err != nil {
			return err
		}
		changed = append(changed, id)
	}
	for id := range old.Upstreams {
		if _, exists := s.Upstreams[id]; !exists {
			changed = append(changed, id)
		}
	}
	if len(changed) > 0 {
		if _, err := t.q.Exec(ctx, `UPDATE upstreams SET invocation_security_revision=$1 WHERE id::text=ANY($2) AND tenant_id=`+tenantSQL, s.Revision+1, changed); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	cipher, err := t.box.SealBound(raw, "invocation-security", "configuration")
	if err != nil {
		return err
	}
	tag, err := t.q.Exec(ctx, `UPDATE invocation_security SET ciphertext=$1,version=version+1 WHERE singleton AND version=$2`, cipher, s.Revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apierror.PreconditionFailed()
	}
	t.targets["invocation-security"] = true
	return nil
}

func retainInvocationCredentials(services, previousServices map[string]invocation.Service) error {
	for name, svc := range services {
		if svc.Token == "" {
			if previous, ok := previousServices[name]; ok && previous.URL != svc.URL {
				return apierror.New(422, "credentials_required", "Credentials required", "Changing an approval endpoint requires an explicit service token")
			}
			svc.Token = previousServices[name].Token
		}
		svc.HasToken = false
		if svc.Token == "" {
			return apierror.New(422, "credentials_required", "Credentials required", "Approval service token required")
		}
		services[name] = svc
	}
	return nil
}

func (t *Tx) validateInvocationContext(ctx context.Context, id string, policy invocation.UpstreamPolicy) error {
	var caller *callercontext.Config
	err := t.q.QueryRow(ctx, `SELECT caller_context FROM upstreams WHERE id::text=$1 AND tenant_id=`+tenantSQL, id).Scan(&caller)
	if err == pgx.ErrNoRows {
		return apierror.NotFound("upstream", id)
	}
	if err != nil {
		return err
	}
	// An inherited caller context is covered by the default rule. Validate
	// explicitly configured tool identities as well.
	rules := []invocation.Rule{policy.Rule}
	for _, rule := range policy.Tools {
		rules = append(rules, rule)
	}
	for _, rule := range rules {
		if rule.Identity == nil || rule.Identity.Source != "caller_context" {
			continue
		}
		if caller == nil || !caller.Enabled {
			return apierror.New(422, "invalid_identity_context", "Invalid identity context", "Enable caller context on the upstream first")
		}
		for _, field := range rule.Identity.Fields {
			if !slices.ContainsFunc(caller.Fields, func(f callercontext.Field) bool { return f.Name == field }) {
				return apierror.New(422, "invalid_identity_context", "Invalid identity context", "Identity fields must be selected by the upstream caller context configuration")
			}
		}
	}
	return nil
}

type SharedInvocationServices struct {
	Revision int64                         `json:"revision"`
	Services map[string]invocation.Service `json:"services"`
	InUse    []string                      `json:"inUse"`
}

func (t *Tx) SharedInvocationServices(ctx context.Context) (SharedInvocationServices, error) {
	settings, err := t.InvocationSecurity(ctx)
	if err != nil {
		return SharedInvocationServices{}, err
	}
	settings.HideCredentials(true)
	used := map[string]struct{}{}
	add := func(name string) {
		if _, exists := settings.Services[name]; exists {
			used[name] = struct{}{}
		}
	}
	collect := func(rule invocation.Rule) {
		if rule.Identity != nil {
			add(rule.Identity.Service)
		}
		for _, name := range rule.ApprovalServices {
			add(name)
		}
	}
	for _, policy := range settings.Upstreams {
		collect(policy.Rule)
		for _, tool := range policy.Tools {
			collect(tool)
		}
	}
	return SharedInvocationServices{
		Revision: settings.Revision, Services: settings.Services, InUse: append([]string{}, slices.Sorted(maps.Keys(used))...),
	}, nil
}

func (t *Tx) SaveSharedInvocationServices(ctx context.Context, revision int64, services map[string]invocation.Service) error {
	settings, err := t.InvocationSecurity(ctx)
	if err != nil {
		return err
	}
	settings.Revision, settings.Services = revision, services
	return t.SaveInvocationSecurity(ctx, settings)
}

// Export uses slugs so declarative policies can be applied to another deployment.
func (t *Tx) ExportInvocationSecurity(ctx context.Context) (*invocation.Settings, error) {
	s, err := t.InvocationSecurity(ctx)
	if err != nil {
		return nil, err
	}
	if len(s.Services) == 0 && len(s.Upstreams) == 0 {
		return nil, nil
	}
	portable := map[string]invocation.UpstreamPolicy{}
	for id, p := range s.Upstreams {
		var slug string
		if err := t.q.QueryRow(ctx, `SELECT slug FROM upstreams WHERE id::text=$1`, id).Scan(&slug); err != nil {
			return nil, err
		}
		portable[slug] = p
	}
	s.Upstreams = portable
	s.HideCredentials(false)
	return &s, nil
}
func (t *Tx) ApplyInvocationSecurity(ctx context.Context, s invocation.Settings) error {
	resolved := map[string]invocation.UpstreamPolicy{}
	for slug, p := range s.Upstreams {
		var id string
		if err := t.q.QueryRow(ctx, `SELECT id::text FROM upstreams WHERE slug=$1 AND tenant_id=`+tenantSQL, slug).Scan(&id); err != nil {
			return apierror.NotFound("upstream", slug)
		}
		resolved[id] = p
	}
	s.Upstreams = resolved
	current, err := t.InvocationSecurity(ctx)
	if err != nil {
		return err
	}
	s.Revision = current.Revision
	return t.SaveInvocationSecurity(ctx, s)
}
