package store

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/callercontext"

	"github.com/bishoku/vmcp/core/internal/apierror"
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
	for name, svc := range s.Services {
		if svc.Token == "" {
			if previous, ok := old.Services[name]; ok && previous.URL != svc.URL {
				return apierror.New(422, "credentials_required", "Credentials required", "Changing an approval endpoint requires an explicit service token")
			}
			svc.Token = old.Services[name].Token
		}
		svc.HasToken = false
		if svc.Token == "" {
			return apierror.New(422, "credentials_required", "Credentials required", "Approval service token required")
		}
		s.Services[name] = svc
	}
	if err = s.Validate(); err != nil {
		return err
	}
	for id := range s.Upstreams {
		var exists bool
		if err = t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM upstreams WHERE id::text=$1 AND tenant_id=`+tenantSQL+`)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apierror.NotFound("upstream", id)
		}
		var caller *callercontext.Config
		if err = t.q.QueryRow(ctx, `SELECT caller_context FROM upstreams WHERE id::text=$1`, id).Scan(&caller); err != nil {
			return err
		}
		policies := []invocation.Rule{s.Upstreams[id].Rule}
		for _, tool := range s.Upstreams[id].Tools {
			effective, e := invocation.Merge(s.Upstreams[id].Rule, tool)
			if e != nil {
				return e
			}
			policies = append(policies, effective)
		}
		for _, rule := range policies {
			if rule.Identity != nil && rule.Identity.Source == "caller_context" {
				if caller == nil || !caller.Enabled {
					return apierror.New(422, "invalid_identity_context", "Invalid identity context", "Enable caller context on the upstream first")
				}
				allowed := map[string]bool{}
				for _, f := range caller.Fields {
					allowed[f.Name] = true
				}
				for _, field := range rule.Identity.Fields {
					if !allowed[field] {
						return apierror.New(422, "invalid_identity_context", "Invalid identity context", "Identity fields must be selected by the upstream caller context configuration")
					}
				}
			}
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
	for name, svc := range s.Services {
		svc.Token = ""
		svc.HasToken = false
		s.Services[name] = svc
	}
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
