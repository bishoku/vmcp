// Package securitydemo is an explicitly development-only approval service and MCP client.
package securitydemo

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/invocation"
)

//go:embed static/*
var assets embed.FS

type entry struct {
	Request  invocation.Request
	Binding  string
	Decision invocation.Decision
	Consumed bool
}
type Server struct {
	Verifier                        *agentauth.Verifier
	Issuer, Origin, Secret, Gateway string
	mu                              sync.Mutex
	operations                      map[string]*entry
	executions                      map[string]int
	Discovery                       string
	ApprovalVerifier                *agentauth.Verifier
	CustomerVerifier                *agentauth.Verifier
	customers                       map[string]*customerSession
	challenges                      map[string]string
}

func New(ctx context.Context, issuer, discovery, origin, secret, gateway string) (*Server, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("demo service secret must be at least 32 characters")
	}
	v, err := agentauth.NewVerifier(ctx, agentauth.Config{Issuer: issuer, DiscoveryURL: discovery, Audience: "vmcp-runtime"})
	if err != nil {
		return nil, err
	}
	av, err := agentauth.NewVerifier(ctx, agentauth.Config{Issuer: issuer, DiscoveryURL: discovery, Audience: "vmcp-approval"})
	if err != nil {
		return nil, err
	}
	cv, err := agentauth.NewVerifier(ctx, agentauth.Config{Issuer: issuer, DiscoveryURL: discovery, Audience: "vmcp-customer"})
	if err != nil {
		return nil, err
	}
	return &Server{CustomerVerifier: cv, Verifier: v, ApprovalVerifier: av, Discovery: discovery, Issuer: issuer, Origin: origin, Secret: secret, Gateway: gateway, operations: map[string]*entry{}}, nil
}
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"issuer": s.Issuer, "clientId": "vmcp-security-demo", "approvalClientId": "vmcp-approval-demo", "customerClientId": "vmcp-customer-demo"})
	})
	tokenURL, _ := url.Parse(s.Discovery + "/protocol/openid-connect/token")
	tokenProxy := httputil.NewSingleHostReverseProxy(tokenURL)
	tokenProxy.Director = func(r *http.Request) {
		r.URL.Scheme = tokenURL.Scheme
		r.URL.Host = tokenURL.Host
		r.URL.Path = tokenURL.Path
		r.URL.RawQuery = ""
		r.Host = tokenURL.Host
		r.Header.Del("Cookie")
		r.Header.Del("Authorization")
		r.Header.Del("Origin")
	}
	mux.Handle("POST /oauth/token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		tokenProxy.ServeHTTP(w, r)
	}))
	mux.HandleFunc("POST /check", s.check)
	mux.HandleFunc("POST /customer-sessions", s.customerSessions)
	mux.HandleFunc("/backend", s.backend)
	mux.HandleFunc("GET /approvals/{id}", s.approval)
	mux.HandleFunc("POST /approvals/{id}", s.approval)
	gateway, _ := url.Parse(s.Gateway)
	proxy := httputil.NewSingleHostReverseProxy(gateway)
	original := proxy.Director
	proxy.Director = func(r *http.Request) {
		original(r)
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/gateway")
		r.Header.Del("Origin")
		r.Header.Del("Cookie")
		r.Header.Del("X-VMCP-Omni-Secret")
		r.Header.Del("X-VMCP-Client-IP")
	}
	mux.Handle("/gateway/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/gateway/mcp/v1/virtual/") && r.URL.Path != "/gateway/mcp/v1/omni" {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	mux.Handle("/application-gateway/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			http.Error(w, "application login required", 401)
			return
		}
		user, err := s.CustomerVerifier.Verify(r.Context(), token)
		if err != nil || user.Claims["azp"] != "vmcp-customer-demo" {
			http.Error(w, "application login required", 401)
			return
		}
		if r.Method == "POST" {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
			if err != nil {
				http.Error(w, "invalid request", 400)
				return
			}
			// Only parse the caller namespace: other metadata can contain scalars.
			var envelope struct {
				Params struct {
					Meta map[string]json.RawMessage `json:"_meta"`
				} `json:"params"`
			}
			if json.Unmarshal(body, &envelope) != nil {
				http.Error(w, "invalid request", 400)
				return
			}
			if raw := envelope.Params.Meta["demo/customer"]; raw != nil {
				var values map[string]string
				if json.Unmarshal(raw, &values) != nil {
					http.Error(w, "invalid customer context", 403)
					return
				}
				s.mu.Lock()
				customer, valid := s.callerCustomer(values)
				valid = valid && customer.User.Subject == user.Subject
				s.mu.Unlock()
				if !valid {
					http.Error(w, "customer session mismatch", 403)
					return
				}
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		key, err := os.ReadFile(os.Getenv("DEMO_AGENT_KEY_FILE"))
		if err != nil {
			http.Error(w, "demo application key unavailable", 503)
			return
		}
		r.URL.Path = strings.Replace(r.URL.Path, "/application-gateway/", "/gateway/", 1)
		if r.URL.Path != "/gateway/mcp/v1/virtual/security-context" {
			http.NotFound(w, r)
			return
		}
		r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(key)))
		proxy.ServeHTTP(w, r)
	}))
	static, _ := fs.Sub(assets, "static")
	mux.Handle("/", http.FileServer(http.FS(static)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self' "+s.Issuer+"; frame-ancestors 'none'; base-uri 'none'")
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.Origin {
			http.Error(w, "origin denied", 403)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func binding(req invocation.Request) string {
	// Assurance can legitimately change after another step-up; consent remains
	// bound to identity, policy, destination, method and immutable arguments.
	req.Phase = ""
	req.Identity.ACR = ""
	req.Identity.AuthTime = 0
	if req.IdentitySource == "caller_context" {
		req.User = nil
	}
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Secret)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	var req invocation.Request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || req.Version != "1" || req.OperationID == "" || (req.Phase != "check" && req.Phase != "consume") {
		http.Error(w, "invalid request", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var verified *invocation.Identity
	if req.IdentitySource == "caller_context" {
		customer, ok := s.callerCustomer(req.CallerContext)
		if !ok {
			reply(w, invocation.Decision{Status: "denied"})
			return
		}
		user := customer.User
		verified = &user
		if !customerAssurance(req.Requirements.StepUp, customer) {
			if s.challenges == nil {
				s.challenges = map[string]string{}
			}
			if previous, exists := s.challenges[req.OperationID]; (exists && previous != req.CallerContext["sessionId"]) || (!exists && len(s.challenges) >= 10000) {
				reply(w, invocation.Decision{Status: "denied"})
				return
			}
			s.challenges[req.OperationID] = req.CallerContext["sessionId"]
			reply(w, invocation.Decision{Status: "step_up_required", ApprovalID: req.OperationID, ExpiresAt: time.Now().Add(5 * time.Minute), URL: s.Origin + "/session.html?challenge=" + url.QueryEscape(req.OperationID)})
			return
		}
	} else if req.IdentitySource == "oauth_subject" && req.User != nil && req.User.Subject != "" {
		user := *req.User
		verified = &user
	} else {
		http.Error(w, "identity source required", 400)
		return
	}
	e := s.operations[req.OperationID]
	if e == nil {
		if req.Phase != "check" || len(s.operations) >= 10000 {
			http.Error(w, "operation unavailable", 409)
			return
		}
		id := rand.Text()
		e = &entry{Request: req, Binding: binding(req), Decision: invocation.Decision{Status: "pending", ApprovalID: id, ExpiresAt: time.Now().Add(5 * time.Minute), URL: s.Origin + "/approve.html?id=" + id}}
		if !req.Requirements.Approval {
			e.Decision.Status = "allowed"
			e.Decision.URL = ""
		}
		s.operations[req.OperationID] = e
	}
	if e.Binding != binding(req) || e.Consumed || !e.Decision.ExpiresAt.After(time.Now()) {
		reply(w, invocation.Decision{Status: "denied"})
		return
	}
	if req.Phase == "consume" {
		if e.Decision.Status != "allowed" {
			reply(w, invocation.Decision{Status: "denied"})
			return
		}
		e.Consumed = true
	}
	decision := e.Decision
	decision.VerifiedUser = verified
	reply(w, decision)
}
func (s *Server) approval(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		http.Error(w, "login required", 401)
		return
	}
	id, err := s.ApprovalVerifier.Verify(r.Context(), token)
	if err != nil {
		http.Error(w, "login required", 401)
		return
	}
	if id.Claims["azp"] != "vmcp-approval-demo" {
		http.Error(w, "approval client required", 403)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var e *entry
	for _, candidate := range s.operations {
		if candidate.Decision.ApprovalID == r.PathValue("id") {
			e = candidate
			break
		}
	}
	var user *invocation.Identity
	if e != nil {
		user = e.Request.User
		if e.Request.IdentitySource == "caller_context" {
			if customer, ok := s.callerCustomer(e.Request.CallerContext); ok {
				user = &customer.User
			} else {
				user = nil
			}
		}
	}
	if e == nil || user == nil || id.Subject != user.Subject || s.ApprovalVerifier.Issuer() != user.Issuer {
		http.NotFound(w, r)
		return
	}
	if e.Consumed || !e.Decision.ExpiresAt.After(time.Now()) {
		http.Error(w, "approval expired or consumed", 409)
		return
	}
	if r.Method == "POST" {
		var body struct {
			Decision string `json:"decision"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil || (body.Decision != "allowed" && body.Decision != "denied") {
			http.Error(w, "invalid decision", 400)
			return
		}
		if e.Decision.Status != "pending" {
			http.Error(w, "already decided", 409)
			return
		}
		e.Decision.Status = body.Decision
	}
	view := e.Request
	view.CallerContext = nil
	view.User = user
	reply(w, map[string]any{"request": view, "decision": e.Decision})
}
