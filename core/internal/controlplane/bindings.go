package controlplane

import (
	"net/http"
	"net/netip"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/store"
)

// bindingWire is the JSON body of binding create/put (and an apply entry).
type bindingWire struct {
	Slug           string             `json:"slug"`
	Description    *string            `json:"description"`
	Conditions     []config.Condition `json:"conditions"`
	VirtualServers []string           `json:"virtualServers"`
	IPAllowlist    []string           `json:"ipAllowlist"`
	RateLimit      *int               `json:"rateLimitPerMinute"`
	Enabled        *bool              `json:"enabled"`
}

func (b *bindingWire) input() store.BindingInput {
	in := store.BindingInput{Conditions: b.Conditions, VirtualServers: b.VirtualServers, IPAllowlist: b.IPAllowlist, RateLimit: b.RateLimit, Enabled: true}
	if b.Description != nil {
		in.Description = *b.Description
	}
	if b.Enabled != nil {
		in.Enabled = *b.Enabled
	}
	return in
}

type bindingPatchWire struct {
	Description    *string             `json:"description"`
	Conditions     *[]config.Condition `json:"conditions"`
	VirtualServers *[]string           `json:"virtualServers"`
	IPAllowlist    *[]string           `json:"ipAllowlist"`
	RateLimit      optionalInt         `json:"rateLimitPerMinute"`
	Enabled        *bool               `json:"enabled"`
}

func (s *Server) listBindings(w http.ResponseWriter, r *http.Request) error {
	limit, after, _, err := pageParams(r)
	if err != nil {
		return err
	}
	items, more, err := s.Store.Reader().ListBindings(r.Context(), after, limit)
	if err != nil {
		return err
	}
	var next *string
	if more {
		next = cursorFor(items[len(items)-1].Slug)
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (s *Server) getBinding(w http.ResponseWriter, r *http.Request) error {
	rd := s.Store.Reader()
	slug := r.PathValue("slug")
	b, err := rd.GetBinding(r.Context(), slug)
	if err != nil {
		return err
	}
	v, err := rd.BindingVersion(r.Context(), slug)
	if err != nil {
		return err
	}
	setETag(w, v)
	return writeJSON(w, http.StatusOK, b)
}

func (s *Server) createBinding(w http.ResponseWriter, r *http.Request) error {
	var body bindingWire
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Slug == "" {
		return apierror.Validation([]apierror.FieldError{required("/slug")})
	}
	return s.writeBinding(w, r, body.Slug, body.input(), nil, true)
}

func (s *Server) putBinding(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	var body bindingWire
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Slug != "" && body.Slug != slug {
		return apierror.Validation([]apierror.FieldError{{Field: "/slug", Code: apierror.CodeValidationFailed, Message: "must match the slug in the URL"}})
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	return s.writeBinding(w, r, slug, body.input(), ifMatch, false)
}

func (s *Server) writeBinding(w http.ResponseWriter, r *http.Request, slug string, in store.BindingInput, ifMatch *int64, createOnly bool) error {
	var (
		b       *store.Binding
		outcome store.Outcome
		version int64
	)
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		if b, outcome, err = tx.PutBinding(r.Context(), slug, in, ifMatch, createOnly); err != nil {
			return err
		}
		version, err = tx.BindingVersion(r.Context(), slug)
		return err
	}); err != nil {
		return err
	}
	setETag(w, version)
	s.setRevision(w, r)
	status := http.StatusOK
	if outcome == store.Created {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v1/identity-bindings/"+slug)
	}
	return writeJSON(w, status, b)
}

func (s *Server) patchBinding(w http.ResponseWriter, r *http.Request) error {
	var body bindingPatchWire
	if err := decode(r, &body); err != nil {
		return err
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	slug := r.PathValue("slug")
	var (
		b   *store.Binding
		ver int64
	)
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		b, err = tx.PatchBinding(r.Context(), slug, store.BindingPatch{
			Description: body.Description, Conditions: body.Conditions, VirtualServers: body.VirtualServers, IPAllowlist: body.IPAllowlist,
			RateLimitSet: body.RateLimit.Set, RateLimit: body.RateLimit.Value, Enabled: body.Enabled,
		}, ifMatch)
		if err != nil {
			return err
		}
		ver, err = tx.BindingVersion(r.Context(), slug)
		return err
	}); err != nil {
		return err
	}
	setETag(w, ver)
	s.setRevision(w, r)
	return writeJSON(w, http.StatusOK, b)
}

func (s *Server) deleteBinding(w http.ResponseWriter, r *http.Request) error {
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		return tx.DeleteBinding(r.Context(), r.PathValue("slug"), ifMatch)
	}); err != nil {
		return err
	}
	s.setRevision(w, r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type evaluateWire struct {
	Claims        map[string]any `json:"claims"`
	VirtualServer string         `json:"virtualServer"`
	ClientIP      string         `json:"clientIp"`
}

// evaluateBindings is a dry run of the data-plane decision for a set of claims, so mappings can be
// debugged without minting a token.
func (s *Server) evaluateBindings(w http.ResponseWriter, r *http.Request) error {
	var body evaluateWire
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Claims == nil {
		return apierror.Validation([]apierror.FieldError{required("/claims")})
	}
	var client *netip.Addr
	if body.ClientIP != "" {
		a, err := netip.ParseAddr(body.ClientIP)
		if err != nil {
			return apierror.Validation([]apierror.FieldError{{Field: "/clientIp", Code: apierror.CodeValidationFailed, Message: "must be an IP address"}})
		}
		a = a.Unmap()
		client = &a
	}
	all, _, err := s.Store.Reader().ListBindings(r.Context(), "", everything)
	if err != nil {
		return err
	}
	type match struct {
		Slug           string   `json:"slug"`
		VirtualServers []string `json:"virtualServers"`
		RateLimit      *int     `json:"rateLimitPerMinute"`
	}
	matches := []match{}
	var enabled []config.Binding
	for _, b := range all {
		if !b.Enabled {
			continue
		}
		cb := config.Binding{Slug: b.Slug, Conditions: b.Conditions, Servers: map[string]bool{}}
		for _, v := range b.VirtualServers {
			cb.Servers[v] = true
		}
		for _, ip := range b.IPAllowlist {
			if p, err := netip.ParsePrefix(ip); err == nil {
				cb.Allowed = append(cb.Allowed, p)
			}
		}
		enabled = append(enabled, cb)
		if agentauth.Match(b.Conditions, body.Claims) {
			matches = append(matches, match{b.Slug, b.VirtualServers, b.RateLimit})
		}
	}
	out := map[string]any{"matches": matches}
	if body.VirtualServer != "" {
		ipOK := func(b config.Binding) bool {
			if len(b.Allowed) == 0 || client == nil {
				return true
			}
			for _, p := range b.Allowed {
				if p.Contains(*client) {
					return true
				}
			}
			return false
		}
		b, outcome := agentauth.Resolve(enabled, body.Claims, body.VirtualServer, ipOK)
		result := map[string]any{"allowed": outcome == agentauth.Allowed}
		switch outcome {
		case agentauth.Allowed:
			result["principal"] = b.Principal()
		case agentauth.NoMatch:
			result["reason"] = "no identity binding matches the claims"
		case agentauth.ServerDenied:
			result["reason"] = "matching bindings do not list this virtual server"
		case agentauth.IPDenied:
			result["reason"] = "client address not allowed"
		}
		out["decision"] = result
	}
	return writeJSON(w, http.StatusOK, out)
}
