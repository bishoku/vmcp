package virtual

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/netip"
	"sort"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/omni"
)

type omniContextKey struct{}
type verifiedOmniIPKey struct{}

// RegisterOmni mounts private gateway-internal routes only when explicitly configured.
// The bridge secret authenticates the gateway component, never the end caller.
// Keep these routes off public ingress and protect transport with TLS/network policy.
func (h *Handler) RegisterOmni(mux *http.ServeMux, secret string) {
	if secret == "" {
		return
	}
	wrap := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-VMCP-Omni-Secret")), []byte(secret)) != 1 {
				http.NotFound(w, r)
				return
			}
			if r.URL.Path != "/internal/omni/catalog" {
				ip, err := netip.ParseAddr(r.Header.Get("X-VMCP-Client-IP"))
				if err != nil {
					http.Error(w, "client address required", 400)
					return
				}
				r = r.Clone(context.WithValue(r.Context(), verifiedOmniIPKey{}, ip.Unmap()))
				r.Header.Del("X-Forwarded-For")
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /internal/omni/catalog", wrap(h.omniCatalog))
	mux.HandleFunc("GET /internal/omni/access", wrap(h.omniAccess))
	mux.HandleFunc("POST /internal/omni/schema", wrap(h.omniSchema))
	mux.HandleFunc("POST /internal/omni/invoke", wrap(h.omniInvoke))
	mux.HandleFunc("POST /internal/omni/audit/begin", wrap(h.omniAuditBegin))
	mux.HandleFunc("POST /internal/omni/audit/complete", wrap(h.omniAuditComplete))
}
func omniTools(v config.VirtualServer) ([]mcp.Tool, error) { return v.ListedTools(0, len(v.Tools)) }

type omniPublishedDocument struct {
	document omni.Document
	hash     string
}
type omniPublishedCatalog struct {
	catalog  omni.Catalog
	byServer map[string][]omniPublishedDocument
}

// Cache only published metadata, never credentials, authorization decisions or schemas.
// Snapshot revisions are immutable; the normal data path does not use this lock.
func (h *Handler) publishedOmni(snap *config.Snapshot) (*omniPublishedCatalog, error) {
	h.omniMu.Lock()
	defer h.omniMu.Unlock()
	if h.omniPublished != nil && h.omniPublished.catalog.Revision == snap.Revision {
		return h.omniPublished, nil
	}
	result := &omniPublishedCatalog{catalog: omni.Catalog{Revision: snap.Revision, Documents: []omni.Document{}}, byServer: map[string][]omniPublishedDocument{}}
	for _, v := range snap.VirtualServers {
		if !v.OmniEnabled {
			continue
		}
		tools, err := omniTools(v)
		if err != nil {
			return nil, err
		}
		for _, t := range tools {
			d := omni.Document{Ref: omni.Ref(v.Slug, t.Name), Server: v.Slug, ServerName: v.Name, Project: v.Project, ServerDescription: v.Description, Name: t.Name, Description: t.Description}
			result.catalog.Documents = append(result.catalog.Documents, d)
			result.byServer[v.Slug] = append(result.byServer[v.Slug], omniPublishedDocument{document: d, hash: d.Hash()})
		}
	}
	sort.Slice(result.catalog.Documents, func(i, j int) bool { return result.catalog.Documents[i].Ref < result.catalog.Documents[j].Ref })
	h.omniPublished = result
	return result, nil
}
func (h *Handler) omniCatalog(w http.ResponseWriter, r *http.Request) {
	snap, ok := h.requestSnapshot(w, r)
	if !ok {
		return
	}
	published, err := h.publishedOmni(snap)
	if err != nil {
		http.Error(w, "catalog unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(published.catalog)
}

// Discard per-server denials during discovery: inaccessible servers are not disclosures
// or separate access attempts. The terminal denial is recorded once below.
type discardResponse struct {
	header http.Header
	status int
}

func (d *discardResponse) Header() http.Header       { return d.header }
func (*discardResponse) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardResponse) WriteHeader(status int)    { d.status = status }
func (h *Handler) omniAccess(w http.ResponseWriter, r *http.Request) {
	snap, ok := h.requestSnapshot(w, r)
	if !ok {
		return
	}
	out := omni.Access{Revision: snap.Revision, Refs: map[string]string{}}
	// A fresh auth-only handler avoids copying locks and suppresses per-server denial audit.
	auth := &Handler{JWT: h.JWT, PublicURL: h.PublicURL, Log: h.Log, TrustedProxies: h.TrustedProxies}
	anonymous := r.Header.Get("Authorization") == ""
	if anonymous && !h.OmniAllowAnonymous {
		w.Header().Set("WWW-Authenticate", `Bearer realm="vmcp"`)
		http.Error(w, "runtime credential required", 401)
		return
	}
	if !anonymous {
		probe := &discardResponse{header: http.Header{}}
		if _, ok := auth.authenticateRuntime(probe, r, config.VirtualServer{Slug: "__omni_auth_probe__"}, snap, false); !ok {
			if probe.status == 401 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="vmcp"`)
			}
			http.Error(w, "invalid or disallowed runtime credential", probe.status)
			return
		}
	}
	matched := false
	for _, v := range snap.VirtualServers {
		if _, ok := h.authenticateOmni(auth, &discardResponse{header: http.Header{}}, r, v, snap); !ok {
			continue
		}
		matched = true
		if r.URL.Query().Get("check_only") == "1" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		if !v.OmniEnabled {
			continue
		}
		published, err := h.publishedOmni(snap)
		if err != nil {
			http.Error(w, "catalog unavailable", 503)
			return
		}
		for _, entry := range published.byServer[v.Slug] {
			out.Refs[entry.document.Ref] = entry.hash
		}
	}
	if !matched {
		h.record(audit.Event{Method: r.Method, Status: "denied", Reason: "no accessible virtual server", ClientIP: h.ipString(r), Action: "omni/discovery"})
		http.Error(w, "no accessible virtual server", 403)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

type omniInput struct {
	Meta        json.RawMessage `json:"_meta,omitempty"`
	AuditTicket string          `json:"_audit_ticket"`
	Ref         string          `json:"tool_ref"`
	Arguments   json.RawMessage `json:"arguments"`
	Version     string          `json:"schema_version"`
}

func (h *Handler) omniTarget(w http.ResponseWriter, r *http.Request) (omniInput, *config.Snapshot, config.VirtualServer, Owner, []byte, bool) {
	var p omniInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&p) != nil {
		http.Error(w, "invalid request", 400)
		return p, nil, config.VirtualServer{}, Owner{}, nil, false
	}
	slug, name, err := omni.ParseRef(p.Ref)
	if err != nil {
		http.Error(w, "invalid tool reference", 400)
		return p, nil, config.VirtualServer{}, Owner{}, nil, false
	}
	snap, ok := h.requestSnapshot(w, r)
	if !ok {
		return p, nil, config.VirtualServer{}, Owner{}, nil, false
	}
	v, ok := snap.VirtualServers[slug]
	if !ok || !v.OmniEnabled {
		http.NotFound(w, r)
		return p, nil, v, Owner{}, nil, false
	}
	owner, ok := h.authenticateOmni(h, w, r, v, snap)
	if !ok {
		return p, nil, v, owner, nil, false
	}
	for i, t := range v.Tools {
		if t.Tool.Name == name {
			ts, err := v.ListedTools(i, i+1)
			if err != nil {
				http.Error(w, "schema unavailable", 503)
				return p, nil, v, owner, nil, false
			}
			b, _ := json.Marshal(ts[0])
			return p, snap, v, owner, b, true
		}
	}
	http.NotFound(w, r)
	return p, nil, v, owner, nil, false
}

// Public access is a DP decision; the private bridge secret cannot grant publication.
func (h *Handler) authenticateOmni(auth *Handler, w http.ResponseWriter, r *http.Request, v config.VirtualServer, snap *config.Snapshot) (Owner, bool) {
	public := h.OmniAllowAnonymous && v.OmniEnabled && !v.AuthRequired
	if public && r.Header.Get("Authorization") == "" {
		return Owner{Mode: "anonymous"}, true
	}
	return auth.authenticateRuntime(w, r, v, snap, !public)
}

func (h *Handler) omniSchema(w http.ResponseWriter, r *http.Request) {
	_, _, _, _, b, ok := h.omniTarget(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(omni.Schema{Tool: b, Version: omni.SchemaVersion(b)})
}
func (h *Handler) omniInvoke(w http.ResponseWriter, r *http.Request) {
	p, snap, v, owner, b, ok := h.omniTarget(w, r)
	if !ok {
		return
	}
	if p.Version != "" && p.Version != omni.SchemaVersion(b) {
		http.Error(w, "schema changed; fetch schema again", 409)
		return
	}
	release, ok := h.acquire(owner, v.Slug, h.ipString(r))
	if !ok {
		http.Error(w, "request concurrency limit exceeded", 429)
		return
	}
	defer release()
	// Invocation-scoped sessions prevent capability/credential sharing between agents.
	sess, err := h.Sessions.Create(v.Slug, mcp.SupportedVersions[0])
	if err != nil {
		http.Error(w, "session capacity unavailable", 503)
		return
	}
	defer h.Sessions.Delete(sess.id)
	sess.owner = owner
	sess.keySlug = owner.Principal
	ctx := audit.WithTrace(withSubject(r.Context(), owner.Subject), r.Header.Get("traceparent"))
	if ticket := p.AuditTicket; ticket != "" {
		parent, valid := h.decodeOmniAudit(r, ticket)
		if !valid || parent.Tool != "invoke_tool" {
			http.Error(w, "invalid invocation audit context", 403)
			return
		}
		ctx = context.WithValue(ctx, omniAuditParentKey{}, parent)
	}
	ctx = context.WithValue(ctx, omniContextKey{}, true)
	ctx = context.WithValue(ctx, authorizationGuardKey{}, func(c context.Context) error {
		current, ok := h.requestSnapshot(w, r.WithContext(c))
		if !ok {
			return errAuthorizationChanged
		}
		if current.Revision != snap.Revision {
			http.Error(w, "configuration changed; retry", 409)
			return errAuthorizationChanged
		}
		verified, ok := h.authenticateOmni(h, w, r.WithContext(c), v, current)
		if !ok {
			return errAuthorizationChanged
		}
		if verified != owner {
			http.Error(w, "authorization changed", 403)
			return errAuthorizationChanged
		}
		return nil
	})
	_, name, _ := omni.ParseRef(p.Ref)
	msg, _ := mcp.NewRequest(mcp.NewIntID(1), mcp.MethodToolsCall, mcp.CallToolParams{Name: name, Arguments: p.Arguments, Meta: p.Meta})
	incoming := r.Header.Clone()
	incoming.Del("X-VMCP-Omni-Secret")
	incoming.Del("X-VMCP-Client-IP")
	h.callTool(ctx, w, msg, v, sess, snap, h.ipString(r), incoming)
}
