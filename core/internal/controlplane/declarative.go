package controlplane

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/policy"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

const apiVersion = "vmcp/v1"

// everything is used where the API pages: export and prune need the full set.
const everything = 1_000_000

type desiredWire struct {
	AuditSettings  *audit.Settings     `json:"auditSettings"`
	APIVersion     string              `json:"apiVersion"`
	Upstreams      []upstreamWire      `json:"upstreams"`
	VirtualServers []virtualServerWire `json:"virtualServers"`
	// Bindings nil means "not managed by this document": nothing is applied or pruned.
	Bindings *[]bindingWire `json:"bindings"`
}

type exportUpstream struct {
	RequestHeaderMappings []callercontext.HeaderMapping `json:"requestHeaderMappings"`
	CallerContext         *callercontext.Config         `json:"callerContext,omitempty"`
	Kind                  string                        `json:"kind"`
	REST                  *restapi.Config               `json:"rest,omitempty"`
	Slug                  string                        `json:"slug"`
	Name                  string                        `json:"name"`
	Description           string                        `json:"description"`
	URL                   string                        `json:"url"`
	FallbackURLs          []string                      `json:"fallbackUrls"`
	Transport             string                        `json:"transport"`
	Enabled               bool                          `json:"enabled"`
	// TLS carries public material only; the client key is never exported.
	TLS *exportTLS `json:"tls,omitempty"`
	// Auth carries no secret; on apply an omitted clientSecret keeps the stored one.
	Auth *oauthtoken.Info `json:"auth,omitempty"`
}

type exportTLS struct {
	CAPEM         string `json:"caPem,omitempty"`
	ServerName    string `json:"serverName,omitempty"`
	ClientCertPEM string `json:"clientCertPem,omitempty"`
}

type exportVirtualServer struct {
	Guidance       *config.Guidance         `json:"guidance"`
	Slug           string                   `json:"slug"`
	Name           string                   `json:"name"`
	Description    string                   `json:"description"`
	Enabled        bool                     `json:"enabled"`
	AuthRequired   bool                     `json:"authRequired"`
	PII            pii.Config               `json:"pii"`
	Policy         policy.Config            `json:"policy"`
	RateLimit      *int                     `json:"rateLimitPerMinute"`
	Tools          []store.ToolSelection    `json:"tools"`
	Catalog        []store.CatalogSelection `json:"catalog"`
	ClientFeatures config.ClientFeatures    `json:"clientFeatures"`
}

// exportConfig emits a document without secrets. Re-applying it keeps stored
// headers because "headers" is absent from every upstream.
func (s *Server) exportConfig(w http.ResponseWriter, r *http.Request) error {
	return s.Store.View(r.Context(), func(rd *store.Tx) error {
		ups, _, err := rd.ListUpstreams(r.Context(), "", everything, nil)
		if err != nil {
			return err
		}
		vss, _, err := rd.ListVirtualServers(r.Context(), "", everything, nil)
		if err != nil {
			return err
		}
		doc := struct {
			AuditSettings  audit.Settings        `json:"auditSettings"`
			APIVersion     string                `json:"apiVersion"`
			Upstreams      []exportUpstream      `json:"upstreams"`
			VirtualServers []exportVirtualServer `json:"virtualServers"`
			Bindings       []bindingWire         `json:"bindings"`
		}{APIVersion: apiVersion, Upstreams: []exportUpstream{}, VirtualServers: []exportVirtualServer{}, Bindings: []bindingWire{}}
		doc.AuditSettings, err = rd.AuditSettings(r.Context())
		if err != nil {
			return err
		}
		for _, u := range ups {
			doc.Upstreams = append(doc.Upstreams, exportUpstream{RequestHeaderMappings: u.RequestHeaderMappings, CallerContext: u.CallerContext, Kind: u.Kind, REST: u.REST, Slug: u.Slug, Name: u.Name, Description: u.Description, URL: u.URL, FallbackURLs: u.FallbackURLs, Transport: u.Transport, Enabled: u.Enabled})
			if u.TLS.CAPEM != "" || u.TLS.ServerName != "" || u.TLS.ClientCertPEM != "" {
				doc.Upstreams[len(doc.Upstreams)-1].TLS = &exportTLS{u.TLS.CAPEM, u.TLS.ServerName, u.TLS.ClientCertPEM}
			}
			if u.Auth.Type != "none" {
				a := u.Auth
				a.HasClientSecret = false // read-model detail, not part of the document
				doc.Upstreams[len(doc.Upstreams)-1].Auth = &a
			}
		}
		for _, v := range vss {
			ev := exportVirtualServer{Guidance: v.Guidance, ClientFeatures: v.ClientFeatures, Slug: v.Slug, Name: v.Name, Description: v.Description, Enabled: v.Enabled, AuthRequired: v.AuthRequired, PII: v.PII, Policy: v.Policy, RateLimit: v.RateLimit, Tools: []store.ToolSelection{}}
			for _, t := range v.Tools {
				ev.Tools = append(ev.Tools, t.ToolSelection)
			}
			ev.Catalog = []store.CatalogSelection{}
			for _, c := range v.Catalog {
				ev.Catalog = append(ev.Catalog, c.CatalogSelection)
			}
			doc.VirtualServers = append(doc.VirtualServers, ev)
		}
		bs, _, err := rd.ListBindings(r.Context(), "", everything)
		if err != nil {
			return err
		}
		for _, b := range bs {
			enabled, desc := b.Enabled, b.Description
			doc.Bindings = append(doc.Bindings, bindingWire{Slug: b.Slug, Description: &desc, Conditions: b.Conditions,
				VirtualServers: b.VirtualServers, IPAllowlist: b.IPAllowlist, RateLimit: b.RateLimit, Enabled: &enabled})
		}
		revision, err := rd.Revision(r.Context())
		if err != nil {
			return err
		}
		w.Header().Set("X-VMCP-Config-Revision", strconv.FormatInt(revision, 10))
		w.Header().Set("X-Config-Revision", strconv.FormatInt(revision, 10))
		return writeJSON(w, http.StatusOK, doc)
	})
}

type change struct {
	Kind   string        `json:"kind"`
	Slug   string        `json:"slug"`
	Action store.Outcome `json:"action"`
}

var errDryRun = errors.New("dry run")

func prefixed(prefix string, errs []apierror.FieldError) []apierror.FieldError {
	for i := range errs {
		errs[i].Field = prefix + errs[i].Field
	}
	return errs
}

// applyConfig makes the configuration match a document, atomically.
func (s *Server) applyConfig(w http.ResponseWriter, r *http.Request) error {
	expected, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	var doc desiredWire
	if err := decode(r, &doc); err != nil {
		return err
	}
	prune := queryBool(r, "prune", false)
	dryRun := queryBool(r, "dryRun", false)
	refresh := queryBool(r, "refreshTools", true)

	// 1. Validate the whole document up front, reporting every problem.
	var errs []apierror.FieldError
	if doc.AuditSettings != nil {
		normalized := doc.AuditSettings.Normalize()
		doc.AuditSettings = &normalized
		if err := doc.AuditSettings.Validate(); err != nil {
			return apierror.New(422, "invalid_audit_settings", "Invalid audit settings", err.Error())
		}
		p, _ := r.Context().Value(principalKey{}).(*principal)
		if p == nil || p.Role != "admin" {
			return apierror.Forbidden("audit settings require admin access")
		}
	}
	if doc.APIVersion != apiVersion {
		errs = append(errs, apierror.FieldError{Field: "/apiVersion", Code: apierror.CodeValidationFailed, Message: fmt.Sprintf("must be %q", apiVersion)})
	}
	upIn := make([]store.UpstreamInput, len(doc.Upstreams))
	upSlugs := map[string]bool{}
	for i, u := range doc.Upstreams {
		p := fmt.Sprintf("/upstreams/%d", i)
		errs = append(errs, prefixed(p, store.SlugErrors("/slug", u.Slug))...)
		if upSlugs[u.Slug] {
			errs = append(errs, apierror.FieldError{Field: p + "/slug", Code: apierror.CodeValidationFailed, Message: "duplicate slug in document"})
		}
		upSlugs[u.Slug] = true
		in, e := u.input()
		errs = append(errs, prefixed(p, e)...)
		if len(e) == 0 {
			errs = append(errs, prefixed(p, store.ValidateUpstreamInput(in))...)
		}
		upIn[i] = in
	}
	vsIn := make([]store.VirtualServerInput, len(doc.VirtualServers))
	vsSlugs := map[string]bool{}
	for i, v := range doc.VirtualServers {
		p := fmt.Sprintf("/virtualServers/%d", i)
		errs = append(errs, prefixed(p, store.SlugErrors("/slug", v.Slug))...)
		if vsSlugs[v.Slug] {
			errs = append(errs, apierror.FieldError{Field: p + "/slug", Code: apierror.CodeValidationFailed, Message: "duplicate slug in document"})
		}
		vsSlugs[v.Slug] = true
		in, e := v.input()
		errs = append(errs, prefixed(p, e)...)
		errs = append(errs, prefixed(p, store.ValidateVirtualServerInput(in))...)
		vsIn[i] = in
	}
	if len(errs) > 0 {
		return apierror.Validation(errs)
	}

	// 2. Fetch tool schemas of new or changed upstreams before opening the transaction,
	//    so no database locks are held while talking to remote servers.
	fetched := map[string][]mcp.Tool{}
	catalogs := map[string]mcp.Catalog{}
	if refresh {
		rd := s.Store.Reader()
		for i, u := range doc.Upstreams {
			in := upIn[i]
			if !in.Enabled || in.Kind == "rest" {
				continue
			}
			cur, err := rd.UpstreamConnInfo(r.Context(), u.Slug)
			if err == nil && cur.Kind == "rest" {
				continue
			}
			var nf *apierror.Error
			isNew := errors.As(err, &nf) && nf.Code == apierror.CodeNotFound
			if err != nil && !isNew {
				return err
			}
			headers := in.Headers
			needs := isNew
			var curTLS tlsconf.Settings
			var curOAuth oauthtoken.Config
			if !isNew {
				curTLS = cur.TLS
				if cur.OAuth != nil {
					curOAuth = *cur.OAuth
				}
			}
			tl, err := store.ResolveTLS(in.TLS, curTLS)
			if err != nil {
				return apierror.PrefixFields(err, fmt.Sprintf("/upstreams/%d", i))
			}
			if !isNew && headers == nil {
				headers = cur.Headers
			}
			oc, err := store.ResolveOAuth(in.OAuth, curOAuth, headers)
			if err != nil {
				return apierror.PrefixFields(err, fmt.Sprintf("/upstreams/%d", i))
			}
			var ocp *oauthtoken.Config
			if !oc.IsZero() {
				ocp = &oc
			}
			if !isNew {
				needs = !store.OAuthEqual(oc, curOAuth) || cur.URL != in.URL || !slices.Equal(cur.FallbackURLs, in.FallbackURLs) || cur.Transport != in.Transport || cur.ToolCount == 0 || !maps.Equal(headers, cur.Headers) || tl != cur.TLS
			}
			if !needs {
				continue
			}
			res, err := probeAny(r.Context(), append([]string{in.URL}, in.FallbackURLs...), in.Transport, headers, tl, ocp)
			if err != nil {
				e := apierror.New(http.StatusUnprocessableEntity, apierror.CodeUpstreamUnreach, "Upstream unreachable",
					fmt.Sprintf("cannot fetch tools of upstream %q: %v", u.Slug, err))
				e.Errors = []apierror.FieldError{{Field: fmt.Sprintf("/upstreams/%d/url", i), Code: apierror.CodeUpstreamUnreach, Message: err.Error()}}
				return e
			}
			fetched[u.Slug] = res.Tools
			catalogs[u.Slug] = res.Catalog
		}
	}

	// 3. Apply everything in one transaction. A dry run rolls it back at the end.
	var changes []change
	err = s.Store.Do(r.Context(), func(tx *store.Tx) error {
		if expected != nil {
			revision, err := tx.LockRevision(r.Context())
			if err != nil {
				return err
			}
			if revision != *expected {
				return apierror.PreconditionFailed()
			}
		}
		ctx := r.Context()
		changes = []change{}
		if doc.AuditSettings != nil {
			current, err := tx.AuditSettings(ctx)
			if err != nil {
				return err
			}
			if err := tx.SaveAuditSettings(ctx, *doc.AuditSettings, current.Revision); err != nil {
				return err
			}
			changes = append(changes, change{"auditSettings", "global", store.Updated})
		}
		now := time.Now().UTC()
		for i, u := range doc.Upstreams {
			_, outcome, err := tx.PutUpstream(ctx, u.Slug, upIn[i], nil, false)
			if err != nil {
				return apierror.PrefixFields(err, fmt.Sprintf("/upstreams/%d", i))
			}
			changes = append(changes, change{"upstream", u.Slug, outcome})
			if tools, ok := fetched[u.Slug]; ok {
				if _, err := tx.ReplaceTools(ctx, u.Slug, tools, now); err != nil {
					return err
				}
				if err := tx.ReplaceCatalog(ctx, u.Slug, catalogs[u.Slug]); err != nil {
					return err
				}
				if err := tx.SetRefreshStatus(ctx, u.Slug, now, nil); err != nil {
					return err
				}
			}
		}
		for j, v := range doc.VirtualServers {
			_, outcome, err := tx.PutVirtualServer(ctx, v.Slug, vsIn[j], nil, false)
			if err != nil {
				return apierror.PrefixFields(err, fmt.Sprintf("/virtualServers/%d", j))
			}
			changes = append(changes, change{"virtualServer", v.Slug, outcome})
		}
		bindingSlugs := map[string]bool{}
		if doc.Bindings != nil {
			for k, b := range *doc.Bindings {
				bindingSlugs[b.Slug] = true
				_, outcome, err := tx.PutBinding(ctx, b.Slug, b.input(), nil, false)
				if err != nil {
					return apierror.PrefixFields(err, fmt.Sprintf("/bindings/%d", k))
				}
				changes = append(changes, change{"identityBinding", b.Slug, outcome})
			}
		}
		if prune {
			if doc.Bindings != nil {
				bs, _, err := tx.ListBindings(ctx, "", everything)
				if err != nil {
					return err
				}
				for _, b := range bs {
					if !bindingSlugs[b.Slug] {
						if err := tx.DeleteBinding(ctx, b.Slug, nil); err != nil {
							return err
						}
						changes = append(changes, change{"identityBinding", b.Slug, store.Deleted})
					}
				}
			}
			vss, _, err := tx.ListVirtualServers(ctx, "", everything, nil)
			if err != nil {
				return err
			}
			for _, v := range vss {
				if !vsSlugs[v.Slug] {
					if err := tx.DeleteVirtualServer(ctx, v.Slug, nil); err != nil {
						return err
					}
					changes = append(changes, change{"virtualServer", v.Slug, store.Deleted})
				}
			}
			ups, _, err := tx.ListUpstreams(ctx, "", everything, nil)
			if err != nil {
				return err
			}
			for _, u := range ups {
				if !upSlugs[u.Slug] {
					// Selections in the document were already validated, so force only
					// removes selections owned by virtual servers that are being pruned.
					if err := tx.DeleteUpstream(ctx, u.Slug, true, nil); err != nil {
						return err
					}
					changes = append(changes, change{"upstream", u.Slug, store.Deleted})
				}
			}
		}
		if dryRun {
			return errDryRun
		}
		return nil
	})
	if err != nil && !errors.Is(err, errDryRun) {
		return err
	}

	var revision *int64
	if !dryRun {
		if rev, err := store.Revision(r.Context(), s.Store.Pool); err == nil {
			revision = &rev
		}
	}
	s.setRevision(w, r)
	return writeJSON(w, http.StatusOK, map[string]any{"dryRun": dryRun, "revision": revision, "changes": changes})
}
