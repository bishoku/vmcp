package controlplane

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/store"
)

type agentKeyWire struct {
	Slug           string     `json:"slug"`
	Description    *string    `json:"description"`
	VirtualServers []string   `json:"virtualServers"`
	IPAllowlist    []string   `json:"ipAllowlist"`
	RateLimit      *int       `json:"rateLimitPerMinute"`
	ExpiresAt      *time.Time `json:"expiresAt"`
}

// optionalTime tells an absent field apart from an explicit null (which clears it).
type optionalTime struct {
	Set   bool
	Value *time.Time
}

func (o *optionalTime) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}

// optionalInt tells an absent field apart from an explicit null (which clears it).
type optionalInt struct {
	Set   bool
	Value *int
}

func (o *optionalInt) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}

type agentKeyPatchWire struct {
	Description    *string      `json:"description"`
	VirtualServers *[]string    `json:"virtualServers"`
	IPAllowlist    *[]string    `json:"ipAllowlist"`
	ExpiresAt      optionalTime `json:"expiresAt"`
	RateLimit      optionalInt  `json:"rateLimitPerMinute"`
}

// withSecret is the one response that carries a key's secret.
type withSecret struct {
	*store.AgentKey
	Key string `json:"key"`
}

func (s *Server) listAgentKeys(w http.ResponseWriter, r *http.Request) error {
	limit, after, _, err := pageParams(r)
	if err != nil {
		return err
	}
	items, more, err := s.Store.Reader().ListAgentKeys(r.Context(), after, limit)
	if err != nil {
		return err
	}
	var next *string
	if more {
		next = cursorFor(items[len(items)-1].Slug)
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (s *Server) getAgentKey(w http.ResponseWriter, r *http.Request) error {
	rd := s.Store.Reader()
	slug := r.PathValue("slug")
	k, err := rd.GetAgentKey(r.Context(), slug)
	if err != nil {
		return err
	}
	ver, err := rd.AgentKeyVersion(r.Context(), slug)
	if err != nil {
		return err
	}
	setETag(w, ver)
	return writeJSON(w, http.StatusOK, k)
}

func (s *Server) createAgentKey(w http.ResponseWriter, r *http.Request) error {
	var body agentKeyWire
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Slug == "" {
		return apierror.Validation([]apierror.FieldError{required("/slug")})
	}
	in := store.AgentKeyInput{VirtualServers: body.VirtualServers, IPAllowlist: body.IPAllowlist, ExpiresAt: body.ExpiresAt, RateLimit: body.RateLimit}
	if body.Description != nil {
		in.Description = *body.Description
	}
	var (
		k      *store.AgentKey
		secret string
	)
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		k, secret, err = tx.CreateAgentKey(r.Context(), body.Slug, in)
		return err
	}); err != nil {
		return err
	}
	s.setRevision(w, r)
	w.Header().Set("Location", "/api/v1/agent-keys/"+body.Slug)
	w.Header().Set("Cache-Control", "no-store")
	return writeJSON(w, http.StatusCreated, withSecret{k, secret})
}

func (s *Server) patchAgentKey(w http.ResponseWriter, r *http.Request) error {
	var body agentKeyPatchWire
	if err := decode(r, &body); err != nil {
		return err
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	slug := r.PathValue("slug")
	var (
		k   *store.AgentKey
		ver int64
	)
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		k, err = tx.PatchAgentKey(r.Context(), slug, store.AgentKeyPatch{
			Description: body.Description, VirtualServers: body.VirtualServers, IPAllowlist: body.IPAllowlist,
			ExpiresAtSet: body.ExpiresAt.Set, ExpiresAt: body.ExpiresAt.Value,
			RateLimitSet: body.RateLimit.Set, RateLimit: body.RateLimit.Value,
		}, ifMatch)
		if err != nil {
			return err
		}
		ver, err = tx.AgentKeyVersion(r.Context(), slug)
		return err
	}); err != nil {
		return err
	}
	setETag(w, ver)
	s.setRevision(w, r)
	return writeJSON(w, http.StatusOK, k)
}

func (s *Server) deleteAgentKey(w http.ResponseWriter, r *http.Request) error {
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		return tx.DeleteAgentKey(r.Context(), r.PathValue("slug"), ifMatch)
	}); err != nil {
		return err
	}
	s.setRevision(w, r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) rotateAgentKey(w http.ResponseWriter, r *http.Request) error {
	var (
		k      *store.AgentKey
		secret string
	)
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		k, secret, err = tx.RotateAgentKey(r.Context(), r.PathValue("slug"))
		return err
	}); err != nil {
		return err
	}
	s.setRevision(w, r)
	w.Header().Set("Cache-Control", "no-store")
	return writeJSON(w, http.StatusOK, withSecret{k, secret})
}

func (s *Server) revokeAgentKey(w http.ResponseWriter, r *http.Request) error {
	var k *store.AgentKey
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		k, err = tx.RevokeAgentKey(r.Context(), r.PathValue("slug"))
		return err
	}); err != nil {
		return err
	}
	s.setRevision(w, r)
	return writeJSON(w, http.StatusOK, k)
}
