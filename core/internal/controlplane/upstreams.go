package controlplane

import (
	"net/http"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

// upstreamWire is the JSON body of upstream create/put (and an apply entry).
type upstreamWire struct {
	CallerContext *callercontext.Config `json:"callerContext"`
	Kind          string                `json:"kind"`
	REST          *restapi.Config       `json:"rest"`
	Slug          string                `json:"slug"`
	Name          *string               `json:"name"`
	Description   *string               `json:"description"`
	URL           *string               `json:"url"`
	FallbackURLs  []string              `json:"fallbackUrls"`
	Transport     *string               `json:"transport"`
	Enabled       *bool                 `json:"enabled"`
	Headers       map[string]string     `json:"headers"`
	TLS           *tlsWire              `json:"tls"`
	Auth          *authWire             `json:"auth"`
}

// tlsWire is the writable TLS block. clientKeyPem is write-only and, when omitted
// together with an unchanged clientCertPem, the stored key is kept.
type tlsWire struct {
	CAPEM         string `json:"caPem"`
	ServerName    string `json:"serverName"`
	ClientCertPEM string `json:"clientCertPem"`
	ClientKeyPEM  string `json:"clientKeyPem"`
}

func (t *tlsWire) settings() *tlsconf.Settings {
	if t == nil {
		return nil
	}
	return &tlsconf.Settings{CAPEM: t.CAPEM, ServerName: t.ServerName, ClientCertPEM: t.ClientCertPEM, ClientKeyPEM: t.ClientKeyPEM}
}

type tlsPatchWire struct {
	CAPEM         *string `json:"caPem"`
	ServerName    *string `json:"serverName"`
	ClientCertPEM *string `json:"clientCertPem"`
	ClientKeyPEM  *string `json:"clientKeyPem"`
}

// authWire is the writable auth block. clientSecret is write-only; omitting it keeps the stored one.
// {"type":"none"} removes OAuth from the upstream.
type authWire struct {
	Type         string   `json:"type"`
	TokenURL     string   `json:"tokenUrl"`
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	Scopes       []string `json:"scopes"`
	Audience     string   `json:"audience"`
}

func (a *authWire) config() *oauthtoken.Config {
	if a == nil {
		return nil
	}
	if a.Type == "none" {
		return &oauthtoken.Config{}
	}
	return &oauthtoken.Config{TokenURL: a.TokenURL, ClientID: a.ClientID, ClientSecret: a.ClientSecret, Scopes: a.Scopes, Audience: a.Audience}
}

type authPatchWire struct {
	Type         *string   `json:"type"`
	TokenURL     *string   `json:"tokenUrl"`
	ClientID     *string   `json:"clientId"`
	ClientSecret *string   `json:"clientSecret"`
	Scopes       *[]string `json:"scopes"`
	Audience     *string   `json:"audience"`
}

func (a *authPatchWire) patch() *store.OAuthPatch {
	if a == nil {
		return nil
	}
	return &store.OAuthPatch{Type: a.Type, TokenURL: a.TokenURL, ClientID: a.ClientID, ClientSecret: a.ClientSecret, Scopes: a.Scopes, Audience: a.Audience}
}

func required(field string) apierror.FieldError {
	return apierror.FieldError{Field: field, Code: apierror.CodeValidationFailed, Message: "is required"}
}

// input applies the API defaults and reports missing required fields.
func (w *upstreamWire) input() (store.UpstreamInput, []apierror.FieldError) {
	in := store.UpstreamInput{CallerContext: w.CallerContext, Kind: w.Kind, REST: w.REST, Transport: "streamable_http", Enabled: true, Headers: w.Headers, FallbackURLs: w.FallbackURLs, TLS: w.TLS.settings(), OAuth: w.Auth.config()}
	var errs []apierror.FieldError
	if w.Name == nil {
		errs = append(errs, required("/name"))
	} else {
		in.Name = *w.Name
	}
	if w.URL == nil {
		errs = append(errs, required("/url"))
	} else {
		in.URL = *w.URL
	}
	if w.Description != nil {
		in.Description = *w.Description
	}
	if w.Transport != nil {
		in.Transport = *w.Transport
	}
	if w.Enabled != nil {
		in.Enabled = *w.Enabled
	}
	return in, errs
}

type upstreamPatchWire struct {
	CallerContext *callercontext.Config `json:"callerContext"`
	Kind          *string               `json:"kind"`
	REST          *restapi.Config       `json:"rest"`
	Name          *string               `json:"name"`
	Description   *string               `json:"description"`
	URL           *string               `json:"url"`
	FallbackURLs  *[]string             `json:"fallbackUrls"`
	Transport     *string               `json:"transport"`
	Enabled       *bool                 `json:"enabled"`
	Headers       map[string]*string    `json:"headers"`
	TLS           *tlsPatchWire         `json:"tls"`
	Auth          *authPatchWire        `json:"auth"`
}

func (s *Server) listUpstreams(w http.ResponseWriter, r *http.Request) error {
	limit, after, enabled, err := pageParams(r)
	if err != nil {
		return err
	}
	items, more, err := s.Store.Reader().ListUpstreams(r.Context(), after, limit, enabled)
	if err != nil {
		return err
	}
	var next *string
	if more {
		next = cursorFor(items[len(items)-1].Slug)
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (s *Server) getUpstream(w http.ResponseWriter, r *http.Request) error {
	rd := s.Store.Reader()
	slug := r.PathValue("slug")
	u, err := rd.GetUpstream(r.Context(), slug)
	if err != nil {
		return err
	}
	v, err := rd.UpstreamVersion(r.Context(), slug)
	if err != nil {
		return err
	}
	setETag(w, v)
	return writeJSON(w, http.StatusOK, u)
}

func (s *Server) createUpstream(w http.ResponseWriter, r *http.Request) error {
	var body upstreamWire
	if err := decode(r, &body); err != nil {
		return err
	}
	in, errs := body.input()
	if body.Slug == "" {
		errs = append(errs, required("/slug"))
	}
	if len(errs) > 0 {
		return apierror.Validation(errs)
	}
	return s.writeUpstream(w, r, body.Slug, in, nil, true)
}

func (s *Server) putUpstream(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	var body upstreamWire
	if err := decode(r, &body); err != nil {
		return err
	}
	in, errs := body.input()
	if body.Slug != "" && body.Slug != slug {
		errs = append(errs, apierror.FieldError{Field: "/slug", Code: apierror.CodeValidationFailed, Message: "must match the slug in the URL"})
	}
	if len(errs) > 0 {
		return apierror.Validation(errs)
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	return s.writeUpstream(w, r, slug, in, ifMatch, false)
}

func (s *Server) writeUpstream(w http.ResponseWriter, r *http.Request, slug string, in store.UpstreamInput, ifMatch *int64, createOnly bool) error {
	var (
		u       *store.Upstream
		outcome store.Outcome
		version int64
	)
	err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		if u, outcome, err = tx.PutUpstream(r.Context(), slug, in, ifMatch, createOnly); err != nil {
			return err
		}
		version, err = tx.UpstreamVersion(r.Context(), slug)
		return err
	})
	if err != nil {
		return err
	}
	setETag(w, version)
	s.setRevision(w, r)
	status := http.StatusOK
	if outcome == store.Created {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v1/upstreams/"+slug)
	}
	return writeJSON(w, status, u)
}

func (s *Server) patchUpstream(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	var body upstreamPatchWire
	if err := decode(r, &body); err != nil {
		return err
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	var (
		u       *store.Upstream
		version int64
	)
	err = s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		u, err = tx.PatchUpstream(r.Context(), slug, store.UpstreamPatch{CallerContext: body.CallerContext, Kind: body.Kind, REST: body.REST,
			Name: body.Name, Description: body.Description, URL: body.URL, FallbackURLs: body.FallbackURLs, Transport: body.Transport,
			Enabled: body.Enabled, Headers: body.Headers, TLS: body.TLS.patch(), OAuth: body.Auth.patch(),
		}, ifMatch)
		if err != nil {
			return err
		}
		version, err = tx.UpstreamVersion(r.Context(), slug)
		return err
	})
	if err != nil {
		return err
	}
	setETag(w, version)
	s.setRevision(w, r)
	return writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUpstream(w http.ResponseWriter, r *http.Request) error {
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	force := queryBool(r, "force", false)
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		return tx.DeleteUpstream(r.Context(), r.PathValue("slug"), force, ifMatch)
	}); err != nil {
		return err
	}
	s.setRevision(w, r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) listUpstreamTools(w http.ResponseWriter, r *http.Request) error {
	tools, err := s.Store.Reader().ListTools(r.Context(), r.PathValue("slug"))
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": tools})
}

func (t *tlsPatchWire) patch() *store.TLSPatch {
	if t == nil {
		return nil
	}
	return &store.TLSPatch{CAPEM: t.CAPEM, ServerName: t.ServerName, ClientCertPEM: t.ClientCertPEM, ClientKeyPEM: t.ClientKeyPEM}
}
