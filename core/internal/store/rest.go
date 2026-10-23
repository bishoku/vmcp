package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/mockupstream"
	"github.com/bishoku/vmcp/core/internal/restapi"
)

// PutUpstream shares the existing transactional connection write and publishes REST
// tool definitions atomically with the encrypted operation configuration.
func (t *Tx) PutUpstream(ctx context.Context, slug string, in UpstreamInput, ifMatch *int64, createOnly bool) (*Upstream, Outcome, error) {
	current, err := t.getUpstream(ctx, slug, true)
	var notFound *apierror.Error
	if err != nil && (!errors.As(err, &notFound) || notFound.Code != apierror.CodeNotFound) {
		return nil, "", err
	}
	kind := in.Kind
	if kind == "" {
		kind = "mcp"
		if current != nil {
			kind = current.Kind
		}
	}
	if kind != "mcp" && kind != "rest" {
		return nil, "", apierror.Validation([]apierror.FieldError{vf("/kind", "must be mcp or rest")})
	}
	if current != nil && kind != current.Kind {
		return nil, "", apierror.Validation([]apierror.FieldError{vf("/kind", "upstream kind is immutable; create a separate upstream")})
	}
	var rest *restapi.Config
	var compiled *restapi.Compiled
	if kind == "rest" {
		var previous *restapi.Config
		if current != nil {
			previous = current.rest
		}
		rest = restapi.Resolve(in.REST, previous)
		headers := in.Headers
		if headers == nil && current != nil {
			headers = current.headers
		}
		if rest == nil {
			return nil, "", apierror.Validation([]apierror.FieldError{vf("/rest", "REST configuration is required")})
		}
		if len(in.FallbackURLs) > 0 || (in.OAuth != nil && !in.OAuth.IsZero()) || (current != nil && !current.oauth.IsZero()) {
			return nil, "", apierror.Validation([]apierror.FieldError{vf("/rest", "REST uses its own auth configuration and does not support fallback URLs")})
		}

		if current != nil && current.rest != nil && (current.rest.Mock != nil) != (rest.Mock != nil) {
			return nil, "", apierror.Validation([]apierror.FieldError{vf("/rest/mock", "mock mode is immutable; create a separate upstream")})
		}
		if rest.Mock != nil {
			if t.wireMockURL == "" && current == nil {
				return nil, "", apierror.Validation([]apierror.FieldError{vf("/rest/mock", "WireMock integration is disabled; configure VMCP_WIREMOCK_URL on the control plane")})
			}
			owner, err := t.MockOwner(ctx)
			if err != nil {
				return nil, "", err
			}
			rest.Mock.Namespace = mockupstream.Namespace(owner, slug)
			if t.wireMockURL != "" {
				in.URL = t.wireMockURL
			} else {
				in.URL = current.URL
			}
		}
		compiled, err = restapi.Compile(in.URL, *rest, headers)
		if err != nil {
			return nil, "", apierror.Validation([]apierror.FieldError{vf("/rest", err.Error())})
		}
	} else if in.REST != nil {
		return nil, "", apierror.Validation([]apierror.FieldError{vf("/rest", "REST operations require a REST upstream")})
	}
	contextConfig := in.CallerContext
	if contextConfig == nil && current != nil {
		contextConfig = current.CallerContext
	}
	headers := in.Headers
	if headers == nil && current != nil {
		headers = current.headers
	}
	static := []map[string]string{headers}
	managed := false
	if rest != nil {
		managed = rest.Auth.Type != "" && rest.Auth.Type != "none"
		for _, op := range rest.Operations {
			static = append(static, op.Headers)
		}
	}
	if err := contextConfig.Validate(kind, static, managed); err != nil {
		return nil, "", apierror.Validation([]apierror.FieldError{vf("/callerContext", err.Error())})
	}
	mappings := in.RequestHeaderMappings
	if mappings == nil && current != nil {
		mappings = current.RequestHeaderMappings
	}
	if err := callercontext.ValidateHeaderMappings(mappings, static, contextConfig, kind); err != nil {
		return nil, "", apierror.Validation([]apierror.FieldError{vf("/requestHeaderMappings", err.Error())})
	}
	result, outcome, err := t.putUpstreamConnection(ctx, slug, in, ifMatch, createOnly)
	if err != nil {
		return nil, "", err
	}
	if contextConfig != nil {
		raw, _ := json.Marshal(contextConfig)
		var old *callercontext.Config
		if current != nil {
			old = current.CallerContext
		}
		oldRaw, _ := json.Marshal(old)
		if !jsonEqual(raw, oldRaw) {
			bump := 0
			if outcome == Unchanged {
				bump = 1
				outcome = Updated
			}
			if _, err = t.q.Exec(ctx, `UPDATE upstreams SET caller_context=$2,version=version+$3,updated_at=now() WHERE tenant_id=`+tenantSQL+` AND slug=$1`, slug, raw, bump); err != nil {
				return nil, "", err
			}
			result, err = t.GetUpstream(ctx, slug)
			if err != nil {
				return nil, "", err
			}
		}
	}
	if mappings != nil {
		raw, _ := json.Marshal(mappings)
		var old []callercontext.HeaderMapping
		if current != nil {
			old = current.RequestHeaderMappings
		}
		oldRaw, _ := json.Marshal(old)
		if !jsonEqual(raw, oldRaw) {
			bump := 0
			if outcome == Unchanged {
				bump = 1
				outcome = Updated
			}
			if _, err = t.q.Exec(ctx, `UPDATE upstreams SET request_header_mappings=$2,version=version+$3,updated_at=now() WHERE tenant_id=`+tenantSQL+` AND slug=$1`, slug, raw, bump); err != nil {
				return nil, "", err
			}
			result, err = t.GetUpstream(ctx, slug)
			if err != nil {
				return nil, "", err
			}
		}
	}
	if kind != "rest" {
		return result, outcome, nil
	}
	oldRaw, _ := json.Marshal(func() *restapi.Config {
		if current != nil {
			return current.rest
		}
		return nil
	}())
	raw, _ := json.Marshal(rest)
	if current == nil || !jsonEqual(oldRaw, raw) {
		var id string
		if err := t.q.QueryRow(ctx, `SELECT id::text FROM upstreams WHERE tenant_id=`+tenantSQL+` AND slug=$1`, slug).Scan(&id); err != nil {
			return nil, "", err
		}
		encrypted, err := t.box.SealBound(raw, id, "upstream.rest_config")
		if err != nil {
			return nil, "", err
		}
		bump := 0
		if outcome == Unchanged {
			bump = 1
			outcome = Updated
		}
		if _, err = t.q.Exec(ctx, `UPDATE upstreams SET kind='rest',encrypted_rest_config=$2,version=version+$3,updated_at=now() WHERE id=$1::uuid`, id, encrypted, bump); err != nil {
			return nil, "", err
		}
	}
	if _, err = t.ReplaceTools(ctx, slug, compiled.Tools, time.Now().UTC()); err != nil {
		return nil, "", err
	}
	result, err = t.GetUpstream(ctx, slug)
	return result, outcome, err
}
