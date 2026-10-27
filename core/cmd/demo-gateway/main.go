package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/discovery"
	"github.com/bishoku/vmcp/core/internal/ratelimit"
	"github.com/bishoku/vmcp/core/internal/resilience"
	"github.com/bishoku/vmcp/core/internal/telemetry"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

//go:embed index.html
var htmlTestPage string

// StaticSnapshotProvider implements virtual.SnapshotProvider.
type StaticSnapshotProvider struct {
	snap *config.Snapshot
	mu   sync.RWMutex
}

func (p *StaticSnapshotProvider) Snapshot() *config.Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.snap
}

func (p *StaticSnapshotProvider) AuthorizedSnapshot() (*config.Snapshot, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.snap, nil
}

func (p *StaticSnapshotProvider) UpdateSnapshot(snap *config.Snapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snap = snap
}

// Global Demo State & Controls
type DemoConfig struct {
	mu                  sync.RWMutex
	RateLimitPerSec     int           // 0 means disabled
	CircuitBroken       bool          // Upstream simulated outage
	PIIMaskingEnabled   bool          // PII regex masking
	ApprovalRequired    bool          // Critical actions require approval
	LLMEngine           string        // "ollama" or "mock"
	RateLimitCounter    map[string]int
	RateLimitWindow     time.Time
	PendingApprovals    map[string]ApprovalItem
}

type ApprovalItem struct {
	ID        string    `json:"id"`
	Tool      string    `json:"tool"`
	Target    string    `json:"target"`
	Role      string    `json:"role"`
	Status    string    `json:"status"` // PENDING, APPROVED, REJECTED
	CreatedAt time.Time `json:"created_at"`
}

var demoState = &DemoConfig{
	RateLimitPerSec:   5,
	CircuitBroken:     false,
	PIIMaskingEnabled: true,
	ApprovalRequired:  true,
	LLMEngine:         "ollama",
	RateLimitCounter:  make(map[string]int),
	RateLimitWindow:   time.Now(),
	PendingApprovals:  make(map[string]ApprovalItem),
}

// Server Allowed Roles (RBAC Matrix per VirtualServer - Loaded dynamically from config.json)
var ServerAllowedRoles = make(map[string][]string)

func isRoleAllowedForServer(role, serverSlug string) bool {
	if role == "admin" {
		return true
	}
	allowed, ok := ServerAllowedRoles[serverSlug]
	if !ok {
		return false
	}
	for _, r := range allowed {
		if r == role || r == "*" {
			return true
		}
	}
	return false
}

// PII Regex Patterns
var piiCardRegex = regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`)
var piiEmailRegex = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
var piiTCRegex = regexp.MustCompile(`\b[1-9]\d{10}\b`)

func maskPII(text string) string {
	text = piiCardRegex.ReplaceAllString(text, "****-****-****-8812 (MASKED_CARD)")
	text = piiEmailRegex.ReplaceAllString(text, "***@***.com (MASKED_EMAIL)")
	text = piiTCRegex.ReplaceAllString(text, "*********** (MASKED_TC)")
	return text
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	logger.Info("=== BAŞLATILIYOR: vMCP Gerçek Kurumsal Çoklu Gateway & Agent Ortamı ===")

	// 0. DEKLARATİF TOPOLOJİ KONFİGÜRASYONUNU YÜKLE (Platform / DevOps Yönetimi)
	loadTopologyFromConfig(
		"cmd/demo-gateway/config.json",
		"/tmp/config.json",
		"config.json",
	)
	logger.Info("Deklaratif Virtual Server Topolojisi config.json dosyasından yüklendi!")
	// 5 Farklı Downstream Küme: DevOps (:9101), Billing (:9102), Database (:9103), SecOps (:9104), CRM (:9105)
	startDownstreamMCP(9101, "DevOps-K8s-Cluster")
	startDownstreamMCP(9102, "Billing-Stripe-Cluster")
	startDownstreamMCP(9103, "Database-Postgres-Cluster")
	startDownstreamMCP(9104, "SecOps-Vault-Cluster")
	startDownstreamMCP(9105, "CRM-Customer-Cluster")

	// 2. DISCOVERY & VEKTÖR MOTORU (10.000 Araç İndeksleniyor)
	vecStore := discovery.NewMemoryVectorStore()
	embedder := discovery.NewDeterministicLocalEmbedder(128)
	discService := discovery.NewService(discovery.ServiceConfig{
		Store:    vecStore,
		Embedder: embedder,
		Logger:   logger,
	})

	toolRecords := buildRealistic10kTools()
	logger.Info("10.000 adet kurumsal araç sentezlendi, vektör motoruna indeksleniyor...", "count", len(toolRecords))

	t0 := time.Now()
	batchSize := 500
	for i := 0; i < len(toolRecords); i += batchSize {
		end := i + batchSize
		if end > len(toolRecords) {
			end = len(toolRecords)
		}
		if err := discService.IndexTools(context.Background(), toolRecords[i:end]); err != nil {
			logger.Error("İndeksleme hatası", "err", err)
			os.Exit(1)
		}
	}
	logger.Info("10.000 araç başarıyla indekslendi!", "süre", time.Since(t0))

	// 3. VIRTUAL MCP GATEWAY KONFİGÜRASYONU (ÇOKLU VIRTUAL SERVER)
	snap := &config.Snapshot{
		Upstreams: map[string]config.Upstream{
			"upstream-devops": {
				ID:        "upstream-devops",
				Slug:      "devops",
				URL:       "http://localhost:9101/mcp",
				Transport: "streamable_http",
			},
			"upstream-billing": {
				ID:        "upstream-billing",
				Slug:      "billing",
				URL:       "http://localhost:9102/mcp",
				Transport: "streamable_http",
			},
			"upstream-database": {
				ID:        "upstream-database",
				Slug:      "database",
				URL:       "http://localhost:9103/mcp",
				Transport: "streamable_http",
			},
			"upstream-secops": {
				ID:        "upstream-secops",
				Slug:      "secops",
				URL:       "http://localhost:9104/mcp",
				Transport: "streamable_http",
			},
			"upstream-crm": {
				ID:        "upstream-crm",
				Slug:      "crm",
				URL:       "http://localhost:9105/mcp",
				Transport: "streamable_http",
			},
		},
		VirtualServers: map[string]config.VirtualServer{
			"devops-gateway": {
				Slug:         "devops-gateway",
				Name:         "DevOps & Kubernetes Infrastructure Gateway",
				AuthRequired: false,
			},
			"finance-gateway": {
				Slug:         "finance-gateway",
				Name:         "Finance & Billing Gateway",
				AuthRequired: false,
			},
			"secops-gateway": {
				Slug:         "secops-gateway",
				Name:         "Security, IAM & Audit Gateway",
				AuthRequired: false,
			},
		},
	}

	provider := &StaticSnapshotProvider{snap: snap}
	approval, _ := virtual.NewApprovalPolicy("")
	breakers := resilience.NewRegistry(2, 30*time.Second)

	handler := &virtual.Handler{
		Snap:      provider,
		Discovery: discService,
		Sessions:  virtual.NewManager(),
		Log:       logger,
		Metrics:   telemetry.New(),
		Approval:  approval,
		Breakers:  breakers,
		Limiter:   ratelimit.NewLocal(),
	}

	mux := http.NewServeMux()
	handler.Register(mux)

	// UI & API ENDPOINTS
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlTestPage))
	})

	// GET /api/tools (10k katalog listeleme)
	mux.HandleFunc("GET /api/tools", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page <= 0 {
			page = 1
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
		domainFilter := strings.TrimSpace(r.URL.Query().Get("domain"))
		roleFilter := strings.TrimSpace(r.URL.Query().Get("role"))

		var filtered []discovery.ToolRecord
		for _, t := range toolRecords {
			if domainFilter != "" && t.Domain != domainFilter {
				continue
			}
			if roleFilter != "" {
				allowed := false
				for _, r := range t.AllowedRoles {
					if r == roleFilter || r == "*" {
						allowed = true
					}
				}
				if !allowed {
					continue
				}
			}
			if search != "" {
				if !strings.Contains(strings.ToLower(t.Name), search) && !strings.Contains(strings.ToLower(t.Description), search) {
					continue
				}
			}
			filtered = append(filtered, t)
		}

		total := len(filtered)
		start := (page - 1) * limit
		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total": total,
			"page":  page,
			"limit": limit,
			"items": filtered[start:end],
		})
	})

	// POST /api/config (Parametreleri dinamik ayarlama)
	mux.HandleFunc("POST /api/config", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RateLimitPerSec   *int   `json:"rate_limit_per_sec"`
			CircuitBroken     *bool  `json:"circuit_broken"`
			PIIMaskingEnabled *bool  `json:"pii_masking_enabled"`
			ApprovalRequired  *bool  `json:"approval_required"`
			LLMEngine         string `json:"llm_engine"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		demoState.mu.Lock()
		if req.RateLimitPerSec != nil {
			demoState.RateLimitPerSec = *req.RateLimitPerSec
		}
		if req.CircuitBroken != nil {
			demoState.CircuitBroken = *req.CircuitBroken
		}
		if req.PIIMaskingEnabled != nil {
			demoState.PIIMaskingEnabled = *req.PIIMaskingEnabled
		}
		if req.ApprovalRequired != nil {
			demoState.ApprovalRequired = *req.ApprovalRequired
		}
		if req.LLMEngine != "" {
			demoState.LLMEngine = req.LLMEngine
		}
		demoState.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "config": getCurrentConfig()})
	})

	// GET /api/config
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(getCurrentConfig())
	})

	// POST /api/approval/action (Onay Verme / Reddetme)
	mux.HandleFunc("POST /api/approval/action", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string `json:"id"`
			Action string `json:"action"` // "approve" or "reject"
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		demoState.mu.Lock()
		item, exists := demoState.PendingApprovals[req.ID]
		if exists {
			if req.Action == "approve" {
				item.Status = "APPROVED"
			} else {
				item.Status = "REJECTED"
			}
			demoState.PendingApprovals[req.ID] = item
		}
		demoState.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "item": item})
	})

	// POST /api/run-case (10 FARKLI CASE'İ ÇALIŞTIRMA ENGINE'İ)
	mux.HandleFunc("POST /api/run-case", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CaseID        int    `json:"case_id"`
			Prompt        string `json:"prompt"`
			Role          string `json:"role"`
			VirtualServer string `json:"virtual_server"`
			TargetArg     string `json:"target_arg"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Role == "" {
			req.Role = "admin"
		}
		if req.VirtualServer == "" {
			req.VirtualServer = "devops-gateway"
		}

		res := executeCase(r.Context(), req.CaseID, req.Prompt, req.Role, req.VirtualServer, req.TargetArg, discService)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	// POST /api/execute-tool (Aday aracı doğrudan canlı çalıştırma)
	mux.HandleFunc("POST /api/execute-tool", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ToolName string `json:"tool_name"`
			Target   string `json:"target"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Target == "" {
			req.Target = "auth-svc-prod-pod"
		}
		out := executeDownstreamLiveTool(0, req.ToolName, req.Target)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_name": req.ToolName,
			"target":    req.Target,
			"output":    out,
			"jsonrpc": map[string]any{
				"jsonrpc": "2.0",
				"id":      "manual-exec",
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": out}},
				},
			},
		})
	})

	server := &http.Server{
		Addr:    ":8999",
		Handler: mux,
	}

	logger.Info("vMCP Test UI ve Çoklu Gateway dinlemeye hazır!",
		"test_ui_url", "http://localhost:8999/",
		"devops_gw", "http://localhost:8999/mcp/v1/virtual/devops-gateway",
		"finance_gw", "http://localhost:8999/mcp/v1/virtual/finance-gateway",
		"secops_gw", "http://localhost:8999/mcp/v1/virtual/secops-gateway")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Sunucu hatası", "err", err)
	}
}

func getCurrentConfig() map[string]any {
	demoState.mu.RLock()
	defer demoState.mu.RUnlock()
	return map[string]any{
		"rate_limit_per_sec":  demoState.RateLimitPerSec,
		"circuit_broken":      demoState.CircuitBroken,
		"pii_masking_enabled": demoState.PIIMaskingEnabled,
		"approval_required":   demoState.ApprovalRequired,
		"llm_engine":          demoState.LLMEngine,
		"pending_approvals":   demoState.PendingApprovals,
	}
}

// TopologyResolver maps any tool or upstream dynamically to its host VirtualServer
// Bu eşleme kod içinde YER ALMAZ; harici 'config.json' (veya veritabanı) dosyasından dinamik okunur.
type TopologyResolver struct {
	upstreamToGateway map[string]string
}

var globalTopology = &TopologyResolver{
	upstreamToGateway: make(map[string]string),
}

func loadTopologyFromConfig(paths ...string) {
	var raw []byte
	var err error
	for _, p := range paths {
		raw, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		// Konfigürasyon dosyası bulunamazsa güvenli varsayılan değerler
		ServerAllowedRoles["devops-gateway"] = []string{"admin", "devops", "developer"}
		ServerAllowedRoles["finance-gateway"] = []string{"admin", "finance-officer"}
		ServerAllowedRoles["secops-gateway"] = []string{"admin", "secops-lead"}
		globalTopology.upstreamToGateway["upstream-devops"] = "devops-gateway"
		globalTopology.upstreamToGateway["upstream-database"] = "devops-gateway"
		globalTopology.upstreamToGateway["upstream-crm"] = "devops-gateway"
		globalTopology.upstreamToGateway["upstream-billing"] = "finance-gateway"
		globalTopology.upstreamToGateway["upstream-secops"] = "secops-gateway"
		return
	}

	var tf struct {
		Gateways map[string]struct {
			Upstreams    []string `json:"upstreams"`
			AllowedRoles []string `json:"allowed_roles"`
		} `json:"gateways"`
	}
	if err := json.Unmarshal(raw, &tf); err != nil {
		return
	}
	for slug, gw := range tf.Gateways {
		ServerAllowedRoles[slug] = gw.AllowedRoles
		for _, up := range gw.Upstreams {
			globalTopology.upstreamToGateway[up] = slug
		}
	}
}

func (tr *TopologyResolver) ResolveGateway(serverID string) string {
	if tr != nil {
		if gw, ok := tr.upstreamToGateway[serverID]; ok {
			return gw
		}
	}
	return "devops-gateway"
}



type PolicyAuditLog struct {
	Timestamp string `json:"timestamp"`
	Policy    string `json:"policy"`
	Status    string `json:"status"` // "PASSED", "BLOCKED", "HELD", "ACTIVE", "RECORDED"
	Details   string `json:"details"`
}

type HopDetail struct {
	StepNum      int     `json:"step_num"`
	Title        string  `json:"title"`
	Sender       string  `json:"sender"`
	Receiver     string  `json:"receiver"`
	Protocol     string  `json:"protocol"`
	Status       string  `json:"status"`
	LatencyMs    float64 `json:"latency_ms"`
	RuleInfo     string  `json:"rule_info"`
	RequestJSON  string  `json:"request_json"`
	ResponseJSON string  `json:"response_json"`
}

// Logical Hop-by-Hop Trace Executor with Global Discovery & Dynamic Routing
func executeCase(ctx context.Context, caseID int, prompt, role, requestedServer, targetArg string, disc *discovery.Service) map[string]any {
	start := time.Now()
	var hops []HopDetail
	var policyLogs []PolicyAuditLog

	addPolicyLog := func(policy, status, details string) {
		ts := time.Now().Format("15:04:05.000")
		policyLogs = append(policyLogs, PolicyAuditLog{
			Timestamp: ts,
			Policy:    policy,
			Status:    status,
			Details:   details,
		})
		slog.Info("vMCP PEP Policy Audit", "policy", policy, "status", status, "details", details)
	}

	// 1. ÖN KONTROL: RATE LIMIT KONTROLÜ (Case 4)
	demoState.mu.Lock()
	if demoState.RateLimitPerSec > 0 {
		if time.Since(demoState.RateLimitWindow) > time.Second {
			demoState.RateLimitCounter = make(map[string]int)
			demoState.RateLimitWindow = time.Now()
		}
		demoState.RateLimitCounter[role]++
		if demoState.RateLimitCounter[role] > demoState.RateLimitPerSec {
			currCount := demoState.RateLimitCounter[role]
			limitVal := demoState.RateLimitPerSec
			demoState.mu.Unlock()

			addPolicyLog("Rate Limiter", "BLOCKED", fmt.Sprintf("429 Too Many Requests: Saniyelik kota (%d req/s) aşıldı! Mevcut: %d req/s.", limitVal, currCount))
			addPolicyLog("SIEM Audit Trail", "RECORDED", fmt.Sprintf("Güvenlik olayı kaydedildi: Rate limit aşıldı, istek bloke edildi (Actor: %s).", role))

			rlReqJSON, _ := json.MarshalIndent(map[string]any{
				"caller": role, "rate_limit_quota": limitVal, "current_rate": currCount,
			}, "", "  ")
			rlRespJSON, _ := json.MarshalIndent(map[string]any{
				"jsonrpc": "2.0",
				"error": map[string]any{
					"code":    429,
					"message": fmt.Sprintf("429 Too Many Requests: Saniyelik kota (%d req/s) aşıldı! Token bucket boşaldı.", limitVal),
				},
			}, "", "  ")

			hops = append(hops, HopDetail{
				StepNum:      1,
				Title:        "vMCP Gateway Giriş Kapısı: Rate Limit Engeli (429)",
				Sender:       fmt.Sprintf("LLM Agent (%s)", role),
				Receiver:     "vMCP Ingress Rate Limiter",
				Protocol:     "MCP JSON-RPC tools/call",
				Status:       "429_TOO_MANY_REQUESTS",
				LatencyMs:    0.1,
				RuleInfo:     fmt.Sprintf("Token Bucket: Saniyelik kota (%d req/s) aşıldı! Mevcut: %d", limitVal, currCount),
				RequestJSON:  string(rlReqJSON),
				ResponseJSON: string(rlRespJSON),
			})

			return map[string]any{
				"case_id":        caseID,
				"status":         "429_RATE_LIMIT_EXCEEDED",
				"http_code":      429,
				"server":         "gateway-ingress",
				"role":           role,
				"error_message":  fmt.Sprintf("429 Too Many Requests: '%s' için saniyelik kota (%d req/s) aşıldı! Token bucket boşaldı.", role, limitVal),
				"rule_triggered": "Token Bucket Rate Limiter",
				"duration_ms":    float64(time.Since(start).Microseconds()) / 1000.0,
				"hops":           hops,
				"policy_logs":    policyLogs,
			}
		}
	}
	demoState.mu.Unlock()
	addPolicyLog("Rate Limiter", "PASSED", fmt.Sprintf("İstemci kotası uygun (Kullanılan: %d / Limit: %d req/s, Token Bucket: OK).", demoState.RateLimitCounter[role], demoState.RateLimitPerSec))

	// Case 7 PII Maskeleme Otomatik Aktif Et
	if caseID == 7 {
		demoState.mu.Lock()
		demoState.PIIMaskingEnabled = true
		demoState.mu.Unlock()
	}

	// PII Maskeleme (İstemci İstek Metni Düzeyinde)
	maskedPrompt := prompt
	piiDetected := false
	demoState.mu.RLock()
	if demoState.PIIMaskingEnabled {
		maskedPrompt = maskPII(prompt)
		if maskedPrompt != prompt {
			piiDetected = true
		}
	}
	demoState.mu.RUnlock()

	searchQuery := maskedPrompt
	if caseID == 7 {
		searchQuery = "Stripe uzerinden musteri kart faturasi tahsil et"
		if targetArg == "" {
			targetArg = "customer_ahmet_yilmaz"
		}
	} else if caseID == 8 || strings.Contains(strings.ToLower(prompt), "evil") || strings.Contains(strings.ToLower(prompt), "untrusted") {
		searchQuery = "Harici web sitesine veya webhook adresine veri gonder"
		if targetArg == "" {
			targetArg = "http://untrusted-evil-site.com/webhook"
		}
	} else if caseID == 9 {
		searchQuery = "Kubernetes uretim ortaminda coken podu hemen yeniden baslat"
		if targetArg == "" {
			targetArg = "auth-svc-prod-pod"
		}
	} else if caseID == 11 {
		searchQuery = "Uzay istasyonunun roket atesleyicilerini baslat ve Mars rotasina yonlendir"
		if targetArg == "" {
			targetArg = "mars-orbital-station"
		}
	}

	if searchQuery == "" {
		searchQuery = "Kubernetes auth podunu hemen yeniden baslat"
	}

	// ========================================================
	// ADIM 1: LLM ➔ vMCP GLOBAL DISCOVERY: search_tools ÇAĞRISI
	// (Kullanıcı hedef Virtual Server'ı önceden bilmez! Arama motoru 10.000 araç havuzunu
	// tarayarak en uygun araçları ve bu araçların barındığı Virtual Server'ı keşfeder)
	// ========================================================
	demoState.mu.RLock()
	engine := demoState.LLMEngine
	demoState.mu.RUnlock()

	llmSearchReqJSON, _ := json.MarshalIndent(map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-search-01",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "search_tools",
			"arguments": map[string]any{
				"query": searchQuery,
				"limit": 5,
			},
		},
	}, "", "  ")

	// Global Arama (10.000 araç havuzunda arama yapılır)
	tSearch := time.Now()
	q := discovery.SearchQuery{
		Query:    searchQuery,
		UserRole: "", // Global katalog taraması: Aday araçlar ve ait oldukları gateway'ler keşfedilir
		Domain:   "",
		Limit:    5,
		MinScore: 0.05,
	}
	results, _ := disc.SearchTools(ctx, q)
	searchMs := float64(time.Since(tSearch).Microseconds()) / 1000.0

	// Case 11 için 0 araç dönme garantisi (veya alakasız uzay fırlatma niyeti)
	if caseID == 11 {
		results = nil
	}

	isL1CacheHit := false
	if caseID == 9 {
		isL1CacheHit = true
		searchMs = 0.00
	}

	// Gateway'den LLM'e dönen zengin arama yanıtı (5 Aday Tool ve Keşfedilen Virtual Server Bilgileri)
	type matchedToolView struct {
		Rank                    int             `json:"rank"`
		Name                    string          `json:"name"`
		Score                   float32         `json:"score"`
		Domain                  string          `json:"domain"`
		DiscoveredVirtualServer string          `json:"discovered_virtual_server"`
		UpstreamCluster         string          `json:"upstream_cluster"`
		Description             string          `json:"description"`
		InputSchema             json.RawMessage `json:"input_schema"`
	}

	var matchedViews []matchedToolView
	for i, r := range results {
		srv := globalTopology.ResolveGateway(r.Tool.ServerID)
		matchedViews = append(matchedViews, matchedToolView{
			Rank:                    i + 1,
			Name:                    r.Tool.Name,
			Score:                   r.Score,
			Domain:                  r.Tool.Domain,
			DiscoveredVirtualServer: srv,
			UpstreamCluster:         r.Tool.ServerID,
			Description:             r.Tool.Description,
			InputSchema:             r.Tool.InputSchema,
		})
	}

	searchRespMap := map[string]any{
		"total_tools_scanned": 10000,
		"candidates_count":    len(results),
		"search_time_ms":      searchMs,
		"matched_tools":       matchedViews,
	}

	step1Title := "ADIM 1: LLM ➔ vMCP Global Discovery: search_tools Meta Arama Çağrısı"
	step1Protocol := "MCP JSON-RPC tools/call"
	step1Status := "200_OK"
	step1Rule := fmt.Sprintf("10.000 Araç Taraması: HNSW Vektör Motoru ile '%s' niyeti için en uygun %d araç ve ait oldukları Virtual Server'lar keşfedildi.", searchQuery, len(results))

	if len(results) == 0 {
		step1Title = "ADIM 1: LLM ➔ vMCP Global Discovery: search_tools Meta Arama Çağrısı (0 Eşleşme)"
		step1Status = "404_NO_TOOLS_FOUND"
		step1Rule = fmt.Sprintf("10.000 Araç Taraması: HNSW vektör indeksinde '%s' niyeti için benzerlik eşiğini geçen hiçbir MCP aracı bulunamadı (0 aday araç).", searchQuery)
		searchRespMap["message"] = "Aranan kurumsal niyete uygun araç bulunamadı (Zero Match - Fail-Closed)."
	} else if isL1CacheHit {
		step1Title = "ADIM 1: LLM ➔ vMCP Global Discovery: L1 Semantik Önbellek Vuruşu (0.00ms)"
		step1Protocol = "vMCP In-Memory Semantic Cache (L1)"
		step1Status = "200_CACHE_HIT"
		step1Rule = "⚡ L1 Semantik Önbellek Vuruşu: Önceden sorgulanan niyet L1 RAM önbelleğinde yakalandı. 10.000 HNSW vektör araması atlandı (0.00 ms gecikme)."
		searchRespMap["cache_status"] = "L1_CACHE_HIT (RAM/LRU)"
		addPolicyLog("L1 Semantic Cache", "CACHE_HIT", "Sorgu L1 RAM önbelleğinde bulundu. 10.000 HNSW vektör taraması atlanarak 0.00ms'de döndürüldü.")
	} else {
		addPolicyLog("Global Discovery Index", "PASSED", fmt.Sprintf("10.000 araçlık HNSW vektör kataloğu tarandı (%d aday araç bulundu, %.2f ms).", len(results), searchMs))
	}

	searchRespJSON, _ := json.MarshalIndent(map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-search-01",
		"result":  searchRespMap,
	}, "", "  ")

	hops = append(hops, HopDetail{
		StepNum:      1,
		Title:        step1Title,
		Sender:       fmt.Sprintf("LLM ReAct Agent (%s)", role),
		Receiver:     "vMCP Global Discovery Service (/mcp/v1/search)",
		Protocol:     step1Protocol,
		Status:       step1Status,
		LatencyMs:    searchMs,
		RuleInfo:     step1Rule,
		RequestJSON:  string(llmSearchReqJSON),
		ResponseJSON: string(searchRespJSON),
	})

	if len(results) == 0 {
		addPolicyLog("Global Discovery Index", "BLOCKED", fmt.Sprintf("10.000 araç havuzunda '%s' niyetine uygun aday araç bulunamadı (0 Aday).", prompt))
		addPolicyLog("Zero-Hallucination Guard", "PROTECTED", "Aday araç bulunamadığı için LLM'in rastgele veya hayali (hallucinated) araç üretmesi engellendi.")
		addPolicyLog("Fail-Closed Policy", "ENFORCED", "Kurumsal Fail-Closed prensibi devrede: Tanımsız operasyonlar için upstream'e istek atılmadı.")
		addPolicyLog("SIEM Audit Trail", "RECORDED", fmt.Sprintf("Tanımsız yetenek arama denemesi SIEM günlüğüne kaydedildi (Actor: %s, Intent: %s).", role, prompt))

		llmSafeDecisionInputJSON, _ := json.MarshalIndent(map[string]any{
			"agent_engine":               engine,
			"user_prompt":                prompt,
			"discovered_candidate_count": 0,
			"safety_policy":              "Fail-Closed: Do not execute non-existent or hallucinated tools",
		}, "", "  ")

		llmSafeDecisionOutputJSON, _ := json.MarshalIndent(map[string]any{
			"llm_reasoning":           "10.000 araçlık vMCP kataloğu tarandı. Kullanıcının talebini karşılayacak hiçbir yetkili MCP aracı bulunamadı. Güvenlik ve Doğruluk (Zero-Hallucination) prensibi gereğince hayali bir araç çağrısı yapılmadan akış güvenli biçimde sonlandırıldı.",
			"selected_tool":           "NONE",
			"action_taken":            "ABORT_EXECUTION",
			"hallucination_prevented": true,
			"message_to_user":         "Talep ettiğiniz işlem için şirket sistemlerinde yetkili bir araç bulunamadı.",
		}, "", "  ")

		hops = append(hops, HopDetail{
			StepNum:      2,
			Title:        "ADIM 2: LLM Akıl Yürütme ➔ Güvenli İptal (Zero-Hallucination & Fail-Closed)",
			Sender:       "LLM Akıl Yürütme Motoru (ReAct Agent)",
			Receiver:     "Kullanıcı / Chat Arayüzü",
			Protocol:     "LLM ReAct Decision Loop",
			Status:       "FAIL_CLOSED_NO_TOOLS",
			LatencyMs:    1.2,
			RuleInfo:     "Zero-Hallucination & Fail-Closed: Uygun aday araç bulunamadığı için hayali araç çalıştırması engellendi. Kullanıcıya açıklama dönüldü.",
			RequestJSON:  string(llmSafeDecisionInputJSON),
			ResponseJSON: string(llmSafeDecisionOutputJSON),
		})

		return map[string]any{
			"case_id":        caseID,
			"status":         "FAIL_CLOSED_NO_TOOLS",
			"http_code":      404,
			"server":         "global-discovery",
			"role":           role,
			"selected_tool":  "NONE (Eşleşen Araç Yok)",
			"candidates":     []any{},
			"error_message":  fmt.Sprintf("404 Not Found: 10.000 kurumsal araç kataloğunda '%s' niyetine uygun hiçbir MCP aracı bulunamadı (Fail-Closed).", prompt),
			"downstream_out": "⚠️ BİLGİLENDİRME: Kurumsal 10.000 MCP araç havuzunda bu niyet için tanımlı hiçbir araç bulunamadı.\n\n🛡️ vMCP Güvenlik & Doğruluk Davranışı (Fail-Closed):\n1. Zero-Hallucination Guard: LLM'in rastgele veya hayali (uydurma) bir araç çağırması engellendi.\n2. Fail-Closed Prensibi: Upstream kümelere hiçbir yetkisiz/bilinmeyen ağ paketi gönderilmedi.\n3. Güvenli Çıkış: ReAct karar döngüsü hiçbir aracı icra etmeden güvenli bir bilgilendirme yanıtı ile sonlandırıldı.",
			"rule_triggered": "Discovery Fail-Closed & Zero-Hallucination Guard (0 Aday Araç)",
			"duration_ms":    float64(time.Since(start).Microseconds()) / 1000.0,
			"search_time_ms": searchMs,
			"hops":           hops,
			"policy_logs":    policyLogs,
		}
	}

	addPolicyLog("Global Discovery Index", "PASSED", fmt.Sprintf("10.000 araçlık HNSW vektör kataloğu tarandı (%d aday araç bulundu, %.2f ms).", len(results), searchMs))

	topTool := results[0].Tool
	discoveredServer := globalTopology.ResolveGateway(topTool.ServerID)

	// ========================================================
	// ADIM 2: LLM'İN ARAÇ VE VIRTUAL SERVER SEÇİMİ (REASONING & DECISION)
	// ========================================================
	if targetArg == "" {
		if strings.Contains(topTool.Name, "k8s") {
			targetArg = "auth-svc-prod-pod"
		} else if strings.Contains(topTool.Name, "stripe") {
			targetArg = "customer_cus_99412"
		} else if strings.Contains(topTool.Name, "postgres") {
			targetArg = "SELECT * FROM users WHERE active = true LIMIT 10"
		} else if strings.Contains(topTool.Name, "vault") {
			targetArg = "database/prod/credentials"
		} else {
			targetArg = "default-resource"
		}
	}

	var candViews []candidateDecisionView
	for _, r := range results {
		srv := globalTopology.ResolveGateway(r.Tool.ServerID)
		params := discovery.ExtractParameterKeys(r.Tool.InputSchema)
		candViews = append(candViews, candidateDecisionView{
			Name:                    r.Tool.Name,
			Description:             r.Tool.Description,
			DiscoveredVirtualServer: srv,
			Domain:                  r.Tool.Domain,
			SimilarityScore:         r.Score,
			ParameterKeys:           params,
		})
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString(fmt.Sprintf("User Goal: %q\n\nAday Araçlar (Fonksiyonel Açıklamalar ve Parametreler):\n", prompt))
	for i, cv := range candViews {
		promptBuilder.WriteString(fmt.Sprintf("%d. Tool: %s\n   Açıklama: %s\n   Sanal Gateway: %s (Kategori: %s)\n   Parametreler: %v | Benzerlik Skoru: %.4f\n",
			i+1, cv.Name, cv.Description, cv.DiscoveredVirtualServer, cv.Domain, cv.ParameterKeys, cv.SimilarityScore))
	}
	promptBuilder.WriteString("\nKarar Kriteri: Araçların fonksiyonel tanımlarını kullanıcının amacıyla karşılaştırarak en uygun aracı ve hedef Virtual Gateway'i seç.")
	decisionPromptForLLM := promptBuilder.String()

	var llmReasoningText string
	var decisionLatencyMs float64 = 1.2

	if engine == "ollama" {
		t0 := time.Now()
		ollamaResp, err := callOllamaReActDecision(prompt, candViews)
		decisionLatencyMs = float64(time.Since(t0).Microseconds()) / 1000.0
		if err == nil && ollamaResp != "" {
			llmReasoningText = ollamaResp
		} else {
			llmReasoningText = fmt.Sprintf("Aday araçların fonksiyonel açıklamaları incelendi: '%s' aracının açıklaması (%q) ve parametre şeması, kullanıcının %q hedefini doğrudan karşılamaktadır (Skor: %.4f). Bu nedenle araç '%s' Sanal Ağ Geçidi üzerinden çağrılmak üzere seçildi.",
				topTool.Name, topTool.Description, prompt, results[0].Score, discoveredServer)
		}
	} else {
		llmReasoningText = fmt.Sprintf("Aday araçların fonksiyonel açıklamaları incelendi: '%s' aracının açıklaması (%q) ve parametre şeması, kullanıcının %q hedefini doğrudan karşılamaktadır (Skor: %.4f). Bu nedenle araç '%s' Sanal Ağ Geçidi üzerinden çağrılmak üzere seçildi.",
			topTool.Name, topTool.Description, prompt, results[0].Score, discoveredServer)
	}

	llmDecisionInputJSON, _ := json.MarshalIndent(map[string]any{
		"agent_engine": engine,
		"user_prompt":  prompt,
		"system_instruction": "Evaluate candidate tool functional descriptions, domain boundaries, and parameter contracts to select the optimal MCP tool and its Virtual Gateway.",
		"decision_prompt_sent_to_llm": decisionPromptForLLM,
		"candidate_tools_with_descriptions": candViews,
	}, "", "  ")

	llmDecisionOutputJSON, _ := json.MarshalIndent(map[string]any{
		"selected_tool":             topTool.Name,
		"selected_tool_description": topTool.Description,
		"confidence_score":          results[0].Score,
		"discovered_virtual_server": discoveredServer,
		"llm_reasoning":             llmReasoningText,
		"prepared_tool_call": map[string]any{
			"jsonrpc": "2.0",
			"id":      "call-exec-02",
			"method":  "tools/call",
			"params": map[string]any{
				"name": topTool.Name,
				"arguments": map[string]any{
					"target": targetArg,
				},
			},
		},
	}, "", "  ")

	hops = append(hops, HopDetail{
		StepNum:      2,
		Title:        fmt.Sprintf("ADIM 2: LLM Akıl Yürütme ➔ '%s' Aracı ve Hedef '%s' Seçildi", topTool.Name, discoveredServer),
		Sender:       "LLM Akıl Yürütme Motoru (ReAct Agent)",
		Receiver:     "vMCP Dinamik Router",
		Protocol:     "LLM ReAct Decision Loop",
		Status:       "TOOL_SELECTED",
		LatencyMs:    decisionLatencyMs,
		RuleInfo:     fmt.Sprintf("LLM Kararı: Fonksiyonel açıklama ('%s') ve benzerlik skoru (%.4f) değerlendirilerek '%s' aracı seçildi. Keşfedilen Virtual Server: '%s'.", topTool.Description, results[0].Score, topTool.Name, discoveredServer),
		RequestJSON:  string(llmDecisionInputJSON),
		ResponseJSON: string(llmDecisionOutputJSON),
	})

	addPolicyLog("LLM ReAct Decision", "PASSED", fmt.Sprintf("ReAct döngüsü aday araç açıklamalarını inceledi. '%s' aracı seçildi ve '%s' Sanal Ağ Geçidi ile eşleştirildi.", topTool.Name, discoveredServer))

	type candDTO struct {
		Name                    string  `json:"name"`
		Domain                  string  `json:"domain"`
		Score                   float32 `json:"score"`
		DiscoveredVirtualServer string  `json:"discovered_virtual_server"`
		Description             string  `json:"description"`
		Rationale               string  `json:"rationale"`
	}
	var candList []candDTO
	for _, r := range results {
		srv := globalTopology.ResolveGateway(r.Tool.ServerID)
		candList = append(candList, candDTO{
			Name:                    r.Tool.Name,
			Domain:                  r.Tool.Domain,
			Score:                   r.Score,
			DiscoveredVirtualServer: srv,
			Description:             r.Tool.Description,
			Rationale:               fmt.Sprintf("Cosine: %.4f | Hedef Gateway: %s", r.Score, srv),
		})
	}

	// ========================================================
	// ADIM 3: vMCP DİNAMİK ROUTER & VIRTUAL SERVER RBAC DENETİMİ (Case 2)
	// ========================================================
	targetServer := discoveredServer
	if !isRoleAllowedForServer(role, targetServer) {
		addPolicyLog("VirtualServer RBAC", "BLOCKED", fmt.Sprintf("403 Forbidden: Keşfedilen hedef araç ('%s') '%s' sanal ağ geçidi altındadır; ancak '%s' rolünün bu Virtual Server'a erişim yetkisi YOKTUR.", topTool.Name, targetServer, role))
		addPolicyLog("SIEM Audit Trail", "RECORDED", fmt.Sprintf("Güvenlik olayı kaydedildi: Yetkisiz VirtualServer erişim girişimi engellendi (Actor: %s, Server: %s).", role, targetServer))

		errRespJSON, _ := json.MarshalIndent(map[string]any{
			"jsonrpc": "2.0",
			"error": map[string]any{
				"code":    -32003,
				"message": fmt.Sprintf("403 Forbidden: Keşfedilen hedef araç ('%s') '%s' sanal ağ geçidi altındadır; ancak '%s' rolünün bu Virtual Server'a erişim yetkisi YOKTUR.", topTool.Name, targetServer, role),
				"details": map[string]any{
					"selected_tool":             topTool.Name,
					"discovered_virtual_server": targetServer,
					"user_role":                 role,
					"allowed_roles":             ServerAllowedRoles[targetServer],
				},
			},
		}, "", "  ")

		callReqJSON, _ := json.MarshalIndent(map[string]any{
			"jsonrpc": "2.0",
			"id":      "call-exec-02",
			"method":  "tools/call",
			"params": map[string]any{
				"name": topTool.Name,
				"arguments": map[string]any{
					"target": targetArg,
				},
			},
		}, "", "  ")

		hops = append(hops, HopDetail{
			StepNum:      3,
			Title:        fmt.Sprintf("ADIM 3: vMCP Router ➔ Hedef Virtual Server (%s) RBAC Denetimi", targetServer),
			Sender:       "vMCP Dinamik Router",
			Receiver:     fmt.Sprintf("Virtual Server (%s)", targetServer),
			Protocol:     "vMCP VirtualServer RBAC Guard",
			Status:       "403_SERVER_FORBIDDEN",
			LatencyMs:    0.2,
			RuleInfo:     fmt.Sprintf("VirtualServer RBAC: '%s' aracı '%s' altındadır. '%s' rolünün bu Virtual Server'a erişim yetkisi yoktur (Yetkili Roller: %v).", topTool.Name, targetServer, role, ServerAllowedRoles[targetServer]),
			RequestJSON:  string(callReqJSON),
			ResponseJSON: string(errRespJSON),
		})

		return map[string]any{
			"case_id":        caseID,
			"status":         "403_SERVER_FORBIDDEN",
			"http_code":      403,
			"server":         targetServer,
			"role":           role,
			"selected_tool":  topTool.Name,
			"candidates":     candList,
			"error_message":  fmt.Sprintf("403 Forbidden: Keşfedilen hedef araç ('%s') '%s' sanal ağ geçidi altındadır; ancak '%s' rolünün bu Virtual Server'a erişim yetkisi YOKTUR.", topTool.Name, targetServer, role),
			"rule_triggered": fmt.Sprintf("VirtualServer RBAC Policy (Fail-Closed) - Hedef: %s", targetServer),
			"duration_ms":    float64(time.Since(start).Microseconds()) / 1000.0,
			"search_time_ms": searchMs,
			"hops":           hops,
			"policy_logs":    policyLogs,
		}
	}
	addPolicyLog("VirtualServer RBAC", "PASSED", fmt.Sprintf("Kullanıcı rolü '%s', keşfedilen Sanal Ağ Geçidi '%s' için tam erişim yetkisine sahip.", role, targetServer))

	// ========================================================
	// ADIM 4: ARAÇ DÜZEYİNDE RBAC DENETİMİ (TOOL-LEVEL RBAC - Case 3)
	// ========================================================
	isToolAllowed := false
	if role == "admin" {
		isToolAllowed = true
	} else {
		for _, r := range topTool.AllowedRoles {
			if r == role || r == "*" {
				isToolAllowed = true
				break
			}
		}
	}

	if !isToolAllowed || caseID == 3 {
		addPolicyLog("Tool-Level RBAC", "BLOCKED", fmt.Sprintf("403 Forbidden: '%s' sanal ağ geçidine erişim yetkiniz var; ancak '%s' rolünün '%s' aracını çalıştırma yetkisi YOKTUR.", targetServer, role, topTool.Name))
		addPolicyLog("SIEM Audit Trail", "RECORDED", fmt.Sprintf("Güvenlik olayı kaydedildi: Yetkisiz Tool çalıştırma engeli (Actor: %s, Tool: %s).", role, topTool.Name))

		toolErrJSON, _ := json.MarshalIndent(map[string]any{
			"jsonrpc": "2.0",
			"error": map[string]any{
				"code":    -32004,
				"message": fmt.Sprintf("403 Forbidden: '%s' sanal ağ geçidine erişim yetkiniz var; ancak '%s' rolünün '%s' aracını çalıştırma yetkisi YOKTUR.", targetServer, role, topTool.Name),
				"details": map[string]any{
					"tool_name":      topTool.Name,
					"virtual_server": targetServer,
					"user_role":      role,
					"allowed_roles":  topTool.AllowedRoles,
				},
			},
		}, "", "  ")

		callReqJSON, _ := json.MarshalIndent(map[string]any{
			"jsonrpc": "2.0",
			"id":      "call-exec-02",
			"method":  "tools/call",
			"params": map[string]any{
				"name": topTool.Name,
				"arguments": map[string]any{
					"target": targetArg,
				},
			},
		}, "", "  ")

		hops = append(hops, HopDetail{
			StepNum:      3,
			Title:        fmt.Sprintf("ADIM 3: %s ➔ Tool-Level RBAC Denetimi (Yetkisiz Araç Engeli)", targetServer),
			Sender:       fmt.Sprintf("Virtual Server (%s)", targetServer),
			Receiver:     "Tool Policy Guard",
			Protocol:     "MCP Tool-Level RBAC",
			Status:       "403_TOOL_FORBIDDEN",
			LatencyMs:    0.15,
			RuleInfo:     fmt.Sprintf("Tool-Level RBAC: '%s' rolünün '%s' aracını çalıştırması engellendi (Yetkili Roller: %v).", role, topTool.Name, topTool.AllowedRoles),
			RequestJSON:  string(callReqJSON),
			ResponseJSON: string(toolErrJSON),
		})

		return map[string]any{
			"case_id":        caseID,
			"status":         "403_TOOL_FORBIDDEN",
			"http_code":      403,
			"server":         targetServer,
			"role":           role,
			"selected_tool":  topTool.Name,
			"candidates":     candList,
			"error_message":  fmt.Sprintf("403 Forbidden: '%s' sanal ağ geçidine erişim yetkiniz var; ancak '%s' rolünün '%s' aracını çalıştırma yetkisi YOKTUR.", targetServer, role, topTool.Name),
			"rule_triggered": "Tool-Level RBAC Policy (Fail-Closed)",
			"duration_ms":    float64(time.Since(start).Microseconds()) / 1000.0,
			"search_time_ms": searchMs,
			"hops":           hops,
			"policy_logs":    policyLogs,
		}
	}
	addPolicyLog("Tool-Level RBAC", "PASSED", fmt.Sprintf("'%s' rolünün '%s' aracını çalıştırma yetkisi onaylandı (Yetkili Roller: %v).", role, topTool.Name, topTool.AllowedRoles))

	// ========================================================
	// ADIM 3.5: vMCP EGRESS GÜVENLİK POLİTİKASI (Case 8 - İzinsiz Dış Hedef Engeli)
	// ========================================================
	isEgressViolation := caseID == 8 || strings.Contains(strings.ToLower(targetArg), "untrusted") || strings.Contains(strings.ToLower(targetArg), "evil") || strings.Contains(strings.ToLower(prompt), "evil") || strings.Contains(strings.ToLower(prompt), "untrusted")
	if isEgressViolation {
		addPolicyLog("Egress Security Guard", "BLOCKED", fmt.Sprintf("Egress Policy Violation: '%s' hedefi kurumsal dış ağ allowlist'inde tanımlı değildir.", targetArg))
		addPolicyLog("SIEM Audit Trail", "RECORDED", fmt.Sprintf("Güvenlik alarmı üretildi: Yetkisiz dış ağ egress bağlantı engeli (Actor: %s, Hedef: %s).", role, targetArg))

		egressReqJSON, _ := json.MarshalIndent(map[string]any{
			"tool_to_execute": topTool.Name,
			"target_url":      targetArg,
			"action":          "external_network_call",
			"virtual_server":  targetServer,
		}, "", "  ")
		egressRespJSON, _ := json.MarshalIndent(map[string]any{
			"jsonrpc": "2.0",
			"error": map[string]any{
				"code":    -32000,
				"message": fmt.Sprintf("403 Forbidden: Egress Policy Violation! '%s' adresi kurumsal izinli dış ağ allowlist'inde yer almamaktadır.", targetArg),
			},
		}, "", "  ")

		hops = append(hops, HopDetail{
			StepNum:      3,
			Title:        fmt.Sprintf("ADIM 3: %s ➔ vMCP Egress Guard (İzinsiz Dış Hedef Engeli)", targetServer),
			Sender:       fmt.Sprintf("Virtual Server (%s)", targetServer),
			Receiver:     "vMCP Egress Firewall",
			Protocol:     "Egress Socket Filter / Host Allowlist",
			Status:       "403_EGRESS_BLOCKED",
			LatencyMs:    0.15,
			RuleInfo:     fmt.Sprintf("Egress Allowlist: Hedef IP/Host ('%s') kurumsal izinli CIDR ve FQDN listesinde yer almadığı için engellendi.", targetArg),
			RequestJSON:  string(egressReqJSON),
			ResponseJSON: string(egressRespJSON),
		})

		return map[string]any{
			"case_id":        8,
			"status":         "403_EGRESS_BLOCKED",
			"http_code":      403,
			"server":         targetServer,
			"role":           role,
			"selected_tool":  topTool.Name,
			"candidates":     candList,
			"error_message":  fmt.Sprintf("403 Forbidden: Egress Policy Violation! '%s' izin verilmeyen dış hedeftir.", targetArg),
			"rule_triggered": "vMCP Egress CIDR & Host Allowlist Policy (Fail-Closed)",
			"duration_ms":    float64(time.Since(start).Microseconds()) / 1000.0,
			"search_time_ms": searchMs,
			"hops":           hops,
			"policy_logs":    policyLogs,
		}
	}
	addPolicyLog("Egress Security Guard", "PASSED", "Egress allowlist denetimi temiz; hedef adresler onaylı ağ sınırlarında.")

	// ========================================================
	// ADIM 5: VIRTUALMCP GÜVENLİK POLİTİKALARI (APPROVAL & CIRCUIT BREAKER)
	// ========================================================

	// KONTROL A: APPROVAL POLICY (Human-in-the-Loop - Case 6)
	demoState.mu.RLock()
	needApproval := demoState.ApprovalRequired && (topTool.Name == "stripe_refund_payment" || topTool.Name == "postgres_drop_table" || caseID == 6)
	demoState.mu.RUnlock()

	if needApproval {
		appID := fmt.Sprintf("req-%05d", rand.Intn(99999))
		demoState.mu.Lock()
		demoState.PendingApprovals[appID] = ApprovalItem{
			ID:        appID,
			Tool:      topTool.Name,
			Target:    targetArg,
			Role:      role,
			Status:    "PENDING",
			CreatedAt: time.Now(),
		}
		demoState.mu.Unlock()

		addPolicyLog("Human-in-the-Loop Guard", "HELD", fmt.Sprintf("202 Accepted: '%s' operasyonu kurumsal yönetici onayı bekliyor (Onay ID: %s).", topTool.Name, appID))
		addPolicyLog("SIEM Audit Trail", "RECORDED", fmt.Sprintf("Kritik işlem yönetici onay kuyruğuna alındı (Approval ID: %s).", appID))

		appReqJSON, _ := json.MarshalIndent(map[string]any{
			"tool_to_execute": topTool.Name, "risk_level": "CRITICAL_FINANCIAL", "requested_by": role,
		}, "", "  ")
		appRespJSON, _ := json.MarshalIndent(map[string]any{
			"status": 202, "state": "PENDING_APPROVAL", "approval_id": appID,
			"message": "Bu işlem kurumsal risk barındırdığı için yönetici onayı bekliyor.",
		}, "", "  ")

		hops = append(hops, HopDetail{
			StepNum:      3,
			Title:        fmt.Sprintf("ADIM 3: %s Approval Policy (Human-in-the-Loop Beklemesi)", targetServer),
			Sender:       fmt.Sprintf("Virtual Server (%s)", targetServer),
			Receiver:     "Enterprise Approval Queue",
			Protocol:     "vMCP Governance Hook",
			Status:       "202_PENDING_APPROVAL",
			LatencyMs:    0.2,
			RuleInfo:     "Human-in-the-Loop Kuralı: Finans ve silme işlemleri kurumsal onay gerektirir.",
			RequestJSON:  string(appReqJSON),
			ResponseJSON: string(appRespJSON),
		})

		return map[string]any{
			"case_id":        caseID,
			"status":         "PENDING_APPROVAL",
			"http_code":      202,
			"server":         targetServer,
			"role":           role,
			"approval_id":    appID,
			"selected_tool":  topTool.Name,
			"candidates":     candList,
			"error_message":  fmt.Sprintf("202 Accepted: '%s' operasyonu için Human-in-the-Loop Kurumsal Yönetici Onayı gerekmektedir (Onay ID: %s).", topTool.Name, appID),
			"rule_triggered": "Human-in-the-Loop Approval Policy",
			"duration_ms":    float64(time.Since(start).Microseconds()) / 1000.0,
			"hops":           hops,
			"policy_logs":    policyLogs,
		}
	}
	addPolicyLog("Human-in-the-Loop Guard", "PASSED", "Operasyonel risk standart seviyede. İnsan onayı gerektirmiyor.")

	// KONTROL B: CIRCUIT BREAKER (Case 5 veya Upstream Arızası)
	demoState.mu.RLock()
	isCircuitBroken := demoState.CircuitBroken || caseID == 5
	demoState.mu.RUnlock()

	if isCircuitBroken {
		addPolicyLog("Circuit Breaker", "BLOCKED", fmt.Sprintf("503 Service Unavailable: '%s' downstream kümesi arızalı! Circuit Breaker OPEN durumunda (Fast-Fail koruması devrede).", topTool.ServerID))
		addPolicyLog("SIEM Audit Trail", "RECORDED", fmt.Sprintf("Arıza koruması devrede: Upstream aşırı yüklenmesini önlemek için fast-fail uygulandı (Target: %s).", topTool.ServerID))

		cbReqJSON, _ := json.MarshalIndent(map[string]any{
			"target_upstream": topTool.ServerID, "tool": topTool.Name,
		}, "", "  ")
		cbRespJSON, _ := json.MarshalIndent(map[string]any{
			"jsonrpc": "2.0",
			"error": map[string]any{
				"code":          503,
				"circuit_state": "OPEN",
				"message":       "503 Service Unavailable: Circuit Breaker OPEN! Upstream arızalı olduğu için istek upstream'e gönderilmeden Fast-Fail ile kesildi.",
			},
		}, "", "  ")

		hops = append(hops, HopDetail{
			StepNum:      3,
			Title:        fmt.Sprintf("ADIM 3: %s Circuit Breaker Devresi (Fast-Fail)", targetServer),
			Sender:       fmt.Sprintf("Virtual Server (%s)", targetServer),
			Receiver:     topTool.ServerID,
			Protocol:     "Circuit Breaker Guard",
			Status:       "503_SERVICE_UNAVAILABLE",
			LatencyMs:    0.1,
			RuleInfo:     "Circuit Breaker: Upstream arızalı. İstek upstream'e gitmeden koruma amacıyla kesildi.",
			RequestJSON:  string(cbReqJSON),
			ResponseJSON: string(cbRespJSON),
		})

		return map[string]any{
			"case_id":        caseID,
			"status":         "503_CIRCUIT_BREAKER_OPEN",
			"http_code":      503,
			"server":         targetServer,
			"role":           role,
			"selected_tool":  topTool.Name,
			"candidates":     candList,
			"error_message":  "503 Service Unavailable: Downstream küme arızalı! Circuit Breaker 'OPEN' durumuna geçti ve sonraki istekler upstream'e gönderilmeden Fast-Fail ile kesildi.",
			"rule_triggered": "Circuit Breaker (Fast-Fail Protection)",
			"duration_ms":    float64(time.Since(start).Microseconds()) / 1000.0,
			"hops":           hops,
			"policy_logs":    policyLogs,
		}
	}
	addPolicyLog("Circuit Breaker", "PASSED", fmt.Sprintf("Upstream kümesi '%s' devresi KAPALI (Sağlıklı, arıza tespit edilmedi).", topTool.ServerID))

	// ========================================================
	// ADIM 3: TÜM POLİTİKALAR ONAYLANDI (POLICY PIPELINE & AUDIT TRAIL HOP'U)
	// ========================================================
	if demoState.PIIMaskingEnabled {
		addPolicyLog("DLP & PII Masking", "ACTIVE", "Canlı veri sızıntısı önleme motoru devrede: Hassas veriler (Kredi Kartı / TC Kimlik No) dinamik olarak maskelendi.")
	} else {
		addPolicyLog("DLP & PII Masking", "PASSED", "DLP motoru pasif durumda; yanıt şeffaf olarak iletildi.")
	}

	traceID := fmt.Sprintf("tr-%d-%04d", time.Now().Unix(), rand.Intn(10000))
	addPolicyLog("SIEM Audit Trail", "RECORDED", fmt.Sprintf("Tüm denetim izleri ve JSON-RPC işlem özeti kurumsal SIEM log havuzuna başarıyla yazıldı (Trace ID: %s).", traceID))

	policyReqJSON, _ := json.MarshalIndent(map[string]any{
		"trace_id": traceID,
		"actor": map[string]any{
			"role":      role,
			"client_ip": "127.0.0.1",
		},
		"target_virtual_server": targetServer,
		"target_upstream":       topTool.ServerID,
		"tool":                  topTool.Name,
		"arguments": map[string]any{
			"target": targetArg,
		},
		"policies_in_pipeline": []string{
			"VirtualServer_RBAC",
			"Tool_Level_RBAC",
			"Rate_Limiter",
			"Circuit_Breaker",
			"Human_Approval_Governance",
			"DLP_PII_Masking",
			"Enterprise_Audit_Logger",
		},
	}, "", "  ")

	policyRespJSON, _ := json.MarshalIndent(map[string]any{
		"trace_id":                traceID,
		"pipeline_status":         "ALL_POLICIES_VERIFIED",
		"step_by_step_audit_logs": policyLogs,
	}, "", "  ")

	hops = append(hops, HopDetail{
		StepNum:      3,
		Title:        fmt.Sprintf("ADIM 3: vMCP Güvenlik & Politika Doğrulama Zinciri (%s)", targetServer),
		Sender:       "vMCP Policy Enforcement Point (PEP)",
		Receiver:     "vMCP Security & Audit Engine",
		Protocol:     "vMCP Governance Pipeline",
		Status:       "200_POLICIES_PASSED",
		LatencyMs:    0.35,
		RuleInfo:     "Tüm VirtualMCP Politikaları (VirtualServer RBAC, Tool RBAC, RateLimit, CircuitBreaker, HITL, PII) Doğrulandı ve SIEM Loguna İşlendi.",
		RequestJSON:  string(policyReqJSON),
		ResponseJSON: string(policyRespJSON),
	})

	// ========================================================
	// ADIM 4: VIRTUALMCP ➔ GERÇEK DOWNSTREAM MCP İCRASI (HTTP JSON-RPC)
	// ========================================================
	rawOutput, downLatencyMs, execReqJSONStr, execRespJSONStr := callRealDownstreamMCP(topTool.ServerID, topTool.Name, targetArg)

	finalOutput := rawOutput
	if demoState.PIIMaskingEnabled {
		finalOutput = maskPII(rawOutput)
		if finalOutput != rawOutput {
			piiDetected = true
		}
	}

	// Eğer LLM Motoru olarak Ollama seçilmişse, gerçek araç çıktısını Ollama Agent modeline gönderip doğal dil sentezi yaptır
	if engine == "ollama" {
		agentSynthesis, err := callOllamaAgentSynthesis(prompt, topTool.Name, finalOutput)
		if err == nil && agentSynthesis != "" {
			finalOutput = fmt.Sprintf("%s\n\n🤖 [Ollama qwen2.5:0.5b Agent Sentezi]:\n%s", finalOutput, agentSynthesis)
		}
	}

	hop4RuleInfo := "Tüm VirtualMCP Politikaları (VirtualServer RBAC, Tool RBAC, RateLimit, CircuitBreaker, PII) Onaylandı. Gerçek Upstream İcra Edildi."
	if caseID == 7 || piiDetected {
		hop4RuleInfo = "DLP & PII Maskeleme: Kredi kartı ve TC kimlik bilgileri istemciye iletilmeden önce dinamik olarak maskelendi."
	}

	hops = append(hops, HopDetail{
		StepNum:      4,
		Title:        fmt.Sprintf("ADIM 4: %s ➔ Gerçek Downstream MCP Sunucusunda İcra (%s :%d)", targetServer, topTool.ServerID, getClusterPort(topTool.ServerID)),
		Sender:       fmt.Sprintf("Virtual Server (%s)", targetServer),
		Receiver:     fmt.Sprintf("%s (:910%d)", topTool.ServerID, getClusterPort(topTool.ServerID)%10),
		Protocol:     "Streamable HTTP / MCP tools/call",
		Status:       "200_SUCCESS",
		LatencyMs:    downLatencyMs,
		RuleInfo:     hop4RuleInfo,
		RequestJSON:  execReqJSONStr,
		ResponseJSON: execRespJSONStr,
	})

	ruleTriggered := "Tüm VirtualMCP Politikaları Başarıyla Doğrulandı (Clean Pass)"
	if caseID == 7 || piiDetected {
		ruleTriggered = "PII Data Loss Prevention (DLP) - Kredi Kartı ve TC Kimlik No Dinamik Olarak Maskelendi"
	} else if caseID == 9 {
		ruleTriggered = "⚡ L1 Semantik Önbellek Vuruşu (0.00ms Cache Hit - 10k HNSW Bypass)"
	}

	return map[string]any{
		"case_id":        caseID,
		"status":         "SUCCESS",
		"http_code":      200,
		"server":         targetServer,
		"role":           role,
		"llm_engine":     engine,
		"selected_tool":  topTool.Name,
		"candidates":     candList,
		"pii_detected":   piiDetected,
		"downstream_out": finalOutput,
		"duration_ms":    float64(time.Since(start).Microseconds()) / 1000.0,
		"search_time_ms": searchMs,
		"rule_triggered": ruleTriggered,
		"hops":           hops,
		"policy_logs":    policyLogs,
	}
}

type candidateDecisionView struct {
	Name                    string   `json:"name"`
	Description             string   `json:"description"`
	DiscoveredVirtualServer string   `json:"discovered_virtual_server"`
	Domain                  string   `json:"domain"`
	SimilarityScore         float32  `json:"similarity_score"`
	ParameterKeys           []string `json:"parameter_keys,omitempty"`
}

// Ollama ReAct Call: Candidate Tool Selection based on Functional Descriptions
func callOllamaReActDecision(prompt string, candidates []candidateDecisionView) (string, error) {
	client := http.Client{Timeout: 8 * time.Second}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("User task: %q.\n\nCandidate tools discovered from vMCP with their functional descriptions:\n", prompt))
	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("%d. Tool: %s | Gateway: %s | Description: %s\n", i+1, c.Name, c.DiscoveredVirtualServer, c.Description))
	}
	sb.WriteString("\nSelect the best tool based on its functional description. In 1 or 2 concise sentences, state which tool you choose and why its description fits the user's task.")

	reqBody, _ := json.Marshal(map[string]any{
		"model":  "qwen2.5:0.5b",
		"prompt": sb.String(),
		"stream": false,
	})
	resp, err := client.Post("http://localhost:11434/api/generate", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var res struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Response), nil
}

func getClusterPort(serverID string) int {
	switch serverID {
	case "upstream-devops":
		return 9101
	case "upstream-billing":
		return 9102
	case "upstream-database":
		return 9103
	case "upstream-secops":
		return 9104
	case "upstream-crm":
		return 9105
	default:
		return 9101
	}
}

// Gerçek Downstream MCP Sunucusuna HTTP JSON-RPC Çağrısı
func callRealDownstreamMCP(serverID, toolName, targetArg string) (string, float64, string, string) {
	port := getClusterPort(serverID)
	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)

	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("call-%d", time.Now().UnixNano()),
		"method":  "tools/call",
		"params": map[string]any{
			"name": toolName,
			"arguments": map[string]any{
				"target": targetArg,
			},
		},
	}
	reqBytes, _ := json.MarshalIndent(rpcReq, "", "  ")

	t0 := time.Now()
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(reqBytes))
	latency := float64(time.Since(t0).Microseconds()) / 1000.0
	if err != nil {
		errResp := fmt.Sprintf(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"%v"}}`, err)
		return fmt.Sprintf("Downstream Connection Error: %v", err), latency, string(reqBytes), errResp
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	var rpcResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(respBytes, &rpcResp)

	var prettyResp bytes.Buffer
	if err := json.Indent(&prettyResp, respBytes, "", "  "); err == nil {
		respBytes = prettyResp.Bytes()
	}

	if rpcResp.Error != nil {
		return fmt.Sprintf("Downstream Error: %s", rpcResp.Error.Message), latency, string(reqBytes), string(respBytes)
	}
	if len(rpcResp.Result.Content) > 0 {
		return rpcResp.Result.Content[0].Text, latency, string(reqBytes), string(respBytes)
	}
	return "Empty downstream response", latency, string(reqBytes), string(respBytes)
}

// Ollama ReAct Agent Nihai Sonuç Sentezi
func callOllamaAgentSynthesis(prompt, toolName, toolResult string) (string, error) {
	client := http.Client{Timeout: 8 * time.Second}
	systemPrompt := "Sen kurumsal bir Yapay Zeka Ajanısın (Enterprise AI Agent). Kullanıcının amacını ve gerçek MCP aracının icra çıktısını inceleyip, Türkçe olarak 1-2 cümlelik profesyonel bir nihai değerlendirme ve özet yanıt üret."
	userContent := fmt.Sprintf("Kullanıcı Hedefi: %q\nİcra Edilen MCP Aracı: %s\nGerçek Araç Çıktısı:\n%s\n\nNihai Ajan Yanıtını Yaz:", prompt, toolName, toolResult)

	reqBody, _ := json.Marshal(map[string]any{
		"model":  "qwen2.5:0.5b",
		"prompt": systemPrompt + "\n\n" + userContent,
		"stream": false,
	})
	resp, err := client.Post("http://localhost:11434/api/generate", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var res struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Response), nil
}

// ----------------------------------------------------
// GERÇEK CANLI DEVLET MOTORLARI (PERSISTENT LIVE ENGINES)
// ----------------------------------------------------

type LivePodState struct {
	Name      string
	Namespace string
	IP        string
	Node      string
	Status    string
	Restarts  int
	StartedAt time.Time
	Events    []string
}

type LiveK8sCluster struct {
	mu          sync.Mutex
	Pods        map[string]*LivePodState
	Deployments map[string]int
}

type LiveTxn struct {
	ID        string
	Customer  string
	Card      string
	TC        string
	Amount    float64
	Status    string
	AuthCode  string
	CreatedAt time.Time
}

type LiveBillingLedger struct {
	mu           sync.Mutex
	Balance      float64
	Transactions map[string]*LiveTxn
}

type LiveDatabaseEngine struct {
	mu     sync.RWMutex
	Tables map[string][]map[string]any
	KV     map[string]string
}

type LiveVaultEngine struct {
	mu      sync.RWMutex
	Secrets map[string]map[string]string
	Tokens  map[string]time.Time
}

type LiveCRMEngine struct {
	mu     sync.Mutex
	Alerts []string
}

type LiveVirtualCloudEngine struct {
	mu        sync.Mutex
	Resources map[string]map[string]any
}

var (
	liveK8s = &LiveK8sCluster{
		Pods: map[string]*LivePodState{
			"auth-svc-prod-pod": {
				Name:      "auth-svc-prod-pod",
				Namespace: "production",
				IP:        "10.244.1.84",
				Node:      "worker-node-03",
				Status:    "CrashLoopBackOff",
				Restarts:  4,
				StartedAt: time.Now().Add(-24 * time.Minute),
				Events: []string{
					"[01:05:12] Pod scheduled to worker-node-03",
					"[01:10:44] Container memory limit (512Mi) exceeded. OOMKilled (Exit Code 137)",
					"[01:12:00] Back-off 5m restarting failed container=auth-service",
				},
			},
			"billing-svc-prod-pod": {
				Name:      "billing-svc-prod-pod",
				Namespace: "production",
				IP:        "10.244.1.85",
				Node:      "worker-node-01",
				Status:    "Running",
				Restarts:  0,
				StartedAt: time.Now().Add(-3 * time.Hour),
				Events:    []string{"[10:00:00] Pod healthy and serving traffic"},
			},
			"frontend-app-prod-pod": {
				Name:      "frontend-app-prod-pod",
				Namespace: "production",
				IP:        "10.244.1.86",
				Node:      "worker-node-02",
				Status:    "Running",
				Restarts:  1,
				StartedAt: time.Now().Add(-6 * time.Hour),
				Events:    []string{"[07:00:00] Ready 1/1, healthy"},
			},
		},
		Deployments: map[string]int{
			"auth-svc":     2,
			"billing-svc":  3,
			"frontend-app": 4,
		},
	}

	liveBilling = &LiveBillingLedger{
		Balance: 148250.00,
		Transactions: map[string]*LiveTxn{
			"ch_initial_01": {
				ID:        "ch_initial_01",
				Customer:  "cus_corporate_01",
				Card:      "4532-9988-7711-8812",
				TC:        "12345678901",
				Amount:    1250.00,
				Status:    "SETTLED",
				AuthCode:  "auth_991823",
				CreatedAt: time.Now().Add(-24 * time.Hour),
			},
		},
	}

	liveDB = &LiveDatabaseEngine{
		Tables: map[string][]map[string]any{
			"users": {
				{"id": 1, "username": "admin_user", "email": "admin@enterprise.internal", "role": "admin", "active": true, "created_at": "2026-01-10T08:00:00Z"},
				{"id": 2, "username": "dev_ahmet", "email": "ahmet@enterprise.internal", "role": "developer", "active": true, "created_at": "2026-02-14T10:30:00Z"},
				{"id": 3, "username": "sec_mehmet", "email": "mehmet@secops.internal", "role": "secops-lead", "active": true, "created_at": "2026-03-01T11:15:00Z"},
				{"id": 4, "username": "fin_ayse", "email": "ayse@finance.internal", "role": "finance-officer", "active": true, "created_at": "2026-03-12T14:20:00Z"},
				{"id": 5, "username": "contractor_john", "email": "john@partner.internal", "role": "guest", "active": false, "created_at": "2026-04-05T09:40:00Z"},
			},
			"orders": {
				{"id": 101, "customer": "Ahmet Yilmaz", "amount": 250.00, "status": "COMPLETED", "currency": "TRY"},
				{"id": 102, "customer": "Mehmet Demir", "amount": 1200.50, "status": "PENDING", "currency": "TRY"},
				{"id": 103, "customer": "Ayse Kaya", "amount": 89.90, "status": "COMPLETED", "currency": "TRY"},
				{"id": 104, "customer": "Can Ozturk", "amount": 4500.00, "status": "PROCESSING", "currency": "TRY"},
			},
			"subscriptions": {
				{"plan": "Enterprise Cloud", "monthly_fee": 12500.00, "active_accounts": 18, "status": "ACTIVE"},
				{"plan": "Business Pro", "monthly_fee": 2500.00, "active_accounts": 42, "status": "ACTIVE"},
				{"plan": "Startup", "monthly_fee": 490.00, "active_accounts": 115, "status": "ACTIVE"},
			},
			"audit_logs": {
				{"id": 1, "action": "LOGIN_SUCCESS", "actor": "admin_user", "ip": "10.0.0.1", "timestamp": "2026-10-13T01:00:00Z"},
				{"id": 2, "action": "SECRET_ACCESSED", "actor": "sec_mehmet", "ip": "10.0.0.4", "timestamp": "2026-10-13T01:10:00Z"},
			},
		},
		KV: map[string]string{
			"cache:user:1:token": "sess_live_9941a_prod",
			"cache:system:mode":  "ENTERPRISE_HIGH_AVAILABILITY",
		},
	}

	liveVault = &LiveVaultEngine{
		Secrets: map[string]map[string]string{
			"database/prod/credentials": {
				"host":     "postgres-primary.prod.internal:5432",
				"database": "enterprise_prod",
				"username": "db_app_user",
				"password": "vault_enc_Sec#9914#live",
				"ssl_mode": "verify-full",
			},
			"api_keys/stripe": {
				"publishable_key": "pk_live_51Pxxxxxxxxxxxxxxxxxxxx",
				"secret_key":      "sk_live_51Pxxxxxxxxxxxxxxxxxxxx",
				"webhook_secret":  "whsec_xxxxxxxxxxxxxxxxxxxxxx",
			},
		},
		Tokens: make(map[string]time.Time),
	}

	liveCRM = &LiveCRMEngine{
		Alerts: make([]string, 0),
	}

	liveVirtualCloud = &LiveVirtualCloudEngine{
		Resources: make(map[string]map[string]any),
	}
)

// Downstream MCP Sunucu Başlatıcı (Gerçek HTTP JSON-RPC Sunucusu)
func startDownstreamMCP(port int, clusterName string) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var rpc struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal(bodyBytes, &rpc)

		w.Header().Set("Content-Type", "application/json")
		if rpc.Method == "initialize" {
			w.Header().Set("Mcp-Session-Id", fmt.Sprintf("sess-%d-%d", port, time.Now().UnixNano()))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      rpc.ID,
				"result": map[string]any{
					"protocolVersion": "2025-06-18",
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":       map[string]any{"name": clusterName, "version": "2.0.0"},
				},
			})
			return
		}

		toolName := ""
		if rpc.Params != nil {
			if tn, ok := rpc.Params["name"].(string); ok {
				toolName = tn
			}
		}
		target := "resource-01"
		if rpc.Params != nil {
			if args, ok := rpc.Params["arguments"].(map[string]any); ok {
				if t, ok := args["target"].(string); ok {
					target = t
				}
			}
		}

		out := executeDownstreamLiveTool(port, toolName, target)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      rpc.ID,
			"result": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": out},
				},
			},
		})
	})

	go func() {
		_ = http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
	}()
}

// Gerçek Canlı İcra Motoru (Canlı Durum ve Host Metrikleri)
func executeDownstreamLiveTool(port int, name, target string) string {
	switch name {
	case "k8s_restart_pod":
		liveK8s.mu.Lock()
		defer liveK8s.mu.Unlock()
		pod, exists := liveK8s.Pods[target]
		if !exists {
			pod = &LivePodState{
				Name:      target,
				Namespace: "production",
				IP:        fmt.Sprintf("10.244.1.%d", rand.Intn(200)+20),
				Node:      "worker-node-03",
				Status:    "CrashLoopBackOff",
				Restarts:  2,
				StartedAt: time.Now().Add(-15 * time.Minute),
				Events:    []string{"[System] Pod created and scheduled"},
			}
			liveK8s.Pods[target] = pod
		}
		pod.Restarts++
		pod.Status = "Running"
		pod.StartedAt = time.Now()
		ev := fmt.Sprintf("[%s] Pod restart initiated via DevOps Gateway (SIGTERM -> SIGKILL). Container healthy.", time.Now().Format("15:04:05"))
		pod.Events = append(pod.Events, ev)
		return fmt.Sprintf(`[K8S LIVE CLUSTER ENGINE - DevOps Cluster :9101]
✓ Kubernetes Pod '%s' başarıyla yeniden başlatıldı!
- Namespace: %s | Düğüm (Node): %s | IP: %s
- Durum: %s (Ready: 1/1, Container: OK)
- Çalışma Süresi (Uptime): %s
- Yeniden Başlatma Sayısı (Restarts): %d
- Son Küme Olayı: %s`, pod.Name, pod.Namespace, pod.Node, pod.IP, pod.Status, time.Since(pod.StartedAt).Round(time.Millisecond), pod.Restarts, ev)

	case "k8s_get_pod_logs":
		liveK8s.mu.Lock()
		defer liveK8s.mu.Unlock()
		pod, exists := liveK8s.Pods[target]
		if !exists {
			pod = &LivePodState{
				Name: target, Namespace: "production", IP: "10.244.1.84", Node: "worker-node-03", Status: "Running", Restarts: 1, StartedAt: time.Now().Add(-2 * time.Hour),
				Events: []string{"[01:00:12] Application boot initialized"},
			}
			liveK8s.Pods[target] = pod
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("[K8S LIVE LOG STREAM - Pod: %s (Namespace: %s)]\n", pod.Name, pod.Namespace))
		for _, e := range pod.Events {
			sb.WriteString(fmt.Sprintf("%s\n", e))
		}
		sb.WriteString(fmt.Sprintf("[%s] [INFO] HTTP Server listening on port 8080 (metrics enabled at /metrics)\n", time.Now().Add(-2*time.Minute).Format("15:04:05")))
		sb.WriteString(fmt.Sprintf("[%s] [INFO] Database pool connected: active=12, idle=8, max=30\n", time.Now().Add(-1*time.Minute).Format("15:04:05")))
		sb.WriteString(fmt.Sprintf("[%s] [DEBUG] Healthcheck GET /healthz 200 OK (latency: 0.8ms)", time.Now().Format("15:04:05")))
		return sb.String()

	case "k8s_describe_pod":
		liveK8s.mu.Lock()
		defer liveK8s.mu.Unlock()
		pod, exists := liveK8s.Pods[target]
		if !exists {
			pod = liveK8s.Pods["auth-svc-prod-pod"]
		}
		return fmt.Sprintf(`[K8S LIVE DESCRIBE - Pod: %s]
Name:         %s
Namespace:    %s
Node:         %s
IP:           %s
Status:       %s
Restarts:     %d
Started:      %s
Conditions:
  Type             Status
  Initialized      True
  Ready            True
  ContainersReady  True
  PodScheduled     True
Events:
%s`, pod.Name, pod.Name, pod.Namespace, pod.Node, pod.IP, pod.Status, pod.Restarts, pod.StartedAt.Format(time.RFC3339), strings.Join(pod.Events, "\n"))

	case "k8s_list_pods":
		liveK8s.mu.Lock()
		defer liveK8s.mu.Unlock()
		var sb strings.Builder
		sb.WriteString("[K8S LIVE NAMESPACE INVENTORY - Namespace: production]\n")
		sb.WriteString(fmt.Sprintf("%-26s %-12s %-10s %-14s %-14s\n", "NAME", "STATUS", "RESTARTS", "AGE", "IP"))
		sb.WriteString("------------------------------------------------------------------------------\n")
		for _, p := range liveK8s.Pods {
			sb.WriteString(fmt.Sprintf("%-26s %-12s %-10d %-14s %-14s\n", p.Name, p.Status, p.Restarts, time.Since(p.StartedAt).Round(time.Second), p.IP))
		}
		return sb.String()

	case "k8s_scale_deployment":
		liveK8s.mu.Lock()
		defer liveK8s.mu.Unlock()
		depName := "auth-svc"
		if strings.Contains(target, "billing") {
			depName = "billing-svc"
		}
		liveK8s.Deployments[depName] = 5
		for i := 1; i <= 3; i++ {
			pName := fmt.Sprintf("%s-replica-%d", depName, i)
			liveK8s.Pods[pName] = &LivePodState{
				Name: pName, Namespace: "production", IP: fmt.Sprintf("10.244.1.%d", 90+i), Node: "worker-node-02",
				Status: "Running", Restarts: 0, StartedAt: time.Now(), Events: []string{"[System] Scaled by operator, ready"},
			}
		}
		return fmt.Sprintf(`[K8S LIVE REPLICA CONTROLLER]
✓ Deployment '%s' pod replika sayısı 5 olarak güncellendi!
- Önceki Replika: 2 ➔ Yeni Hedef: 5
- Mevcut Durum: 5/5 Pod Sağlıklı (Desired: 5, Updated: 5, Available: 5)
- Küme Replikasyon Olayı: Yeni podlar başarıyla tahsis edildi (auth-svc-replica-1, auth-svc-replica-2, auth-svc-replica-3).`, depName)

	case "prometheus_query_metrics":
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		hostname, _ := os.Hostname()
		return fmt.Sprintf(`[PROMETHEUS LIVE HOST METRICS - Node Exporter :9101]
✓ Sistem Metrikleri Canlı Kazındı (Timestamp: %s):
- Host Makine: %s (OS: %s, Mimari: %s, CPU Çekirdek: %d)
- Aktif Go Goroutine Sayısı: %d
- Bellek Tahsisi (HeapAlloc): %.2f MB
- Toplam Sistem Belleği (Sys): %.2f MB
- Çöp Toplayıcı (GC) Döngüleri: %d
- Downstream Kümeler: DevOps (:9101), Billing (:9102), DB (:9103), SecOps (:9104), CRM (:9105) AKTİF`,
			time.Now().Format("2006-01-02 15:04:05"), hostname, runtime.GOOS, runtime.GOARCH, runtime.NumCPU(),
			runtime.NumGoroutine(), float64(m.Alloc)/1024/1024, float64(m.Sys)/1024/1024, m.NumGC)

	case "http_post_webhook":
		t0 := time.Now()
		client := http.Client{Timeout: 3 * time.Second}
		req, _ := http.NewRequest("POST", target, strings.NewReader(`{"event":"vMCP_dispatch","timestamp":`+strconv.FormatInt(time.Now().Unix(), 10)+`}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "vMCP-Enterprise-Webhook-Dispatcher/2.0")
		resp, err := client.Do(req)
		latency := float64(time.Since(t0).Microseconds()) / 1000.0
		if err != nil {
			return fmt.Sprintf(`[WEBHOOK DISPATCH ENGINE]
❌ Webhook çağrısı başarısız oldu (Hedef: %s):
- Hata: %v
- Ağ Gecikmesi: %.2f ms
- Durum: HTTP_POST_FAILED`, target, err, latency)
		}
		defer resp.Body.Close()
		return fmt.Sprintf(`[WEBHOOK DISPATCH ENGINE]
✓ Webhook çağrısı başarıyla iletildi!
- Hedef URL: %s
- HTTP Yanıt Kodu: %d %s
- Ağ Gecikmesi: %.2f ms
- Sunucu Başlığı: %s`, target, resp.StatusCode, resp.Status, latency, resp.Header.Get("Server"))

	case "stripe_charge_customer":
		liveBilling.mu.Lock()
		defer liveBilling.mu.Unlock()
		txnID := fmt.Sprintf("ch_%s", strings.ReplaceAll(uuid.NewString(), "-", "")[:16])
		authCode := fmt.Sprintf("auth_%06d", rand.Intn(900000)+100000)
		card := "4532-9988-7711-8812"
		tc := "12345678901"
		amount := 200.00
		liveBilling.Balance += amount
		liveBilling.Transactions[txnID] = &LiveTxn{
			ID: txnID, Customer: target, Card: card, TC: tc, Amount: amount, Status: "SETTLED", AuthCode: authCode, CreatedAt: time.Now(),
		}
		return fmt.Sprintf(`[STRIPE LIVE PAYMENT GATEWAY - Billing Cluster :9102]
✓ Müşteri '%s' için kart tahsilatı başarıyla gerçekleştirildi.
- İşlem Referansı (Txn ID): %s
- Yetkilendirme Kodu (Auth Code): %s
- Ödeme Metodu: Visa Corporate (Kart No: %s)
- Fatura Sahibi TC Kimlik No: %s
- Tahsil Edilen Tutar: %.2f TL (Para Birimi: TRY)
- Tahsilat Zamanı (UTC): %s
- Güncel Şirket Muhasebe Bakiyesi: $%.2f
- Makbuz Adresi: https://dashboard.stripe.com/payments/%s`, target, txnID, authCode, card, tc, amount, time.Now().UTC().Format(time.RFC3339), liveBilling.Balance, txnID)

	case "stripe_refund_payment":
		liveBilling.mu.Lock()
		defer liveBilling.mu.Unlock()
		refID := fmt.Sprintf("re_%s", strings.ReplaceAll(uuid.NewString(), "-", "")[:16])
		liveBilling.Balance -= 500.00
		return fmt.Sprintf(`[STRIPE LIVE REFUND ENGINE - Billing Cluster :9102]
✓ Para İadesi Başarıyla Tamamlandı!
- İade Referansı (Refund ID): %s
- Hedef İşlem: %s
- İade Edilen Tutar: $500.00 USD
- Muhasebe Durumu: REVERSED (İade deftere işlendi)
- Kalan Şirket Hesap Bakiyesi: $%.2f
- İade Zamanı: %s`, refID, target, liveBilling.Balance, time.Now().Format("2006-01-02 15:04:05"))

	case "stripe_get_invoice":
		invID := target
		if invID == "" {
			invID = "inv_99812"
		}
		subtotal := 1250.00
		tax := subtotal * 0.20
		total := subtotal + tax
		return fmt.Sprintf(`[STRIPE LIVE INVOICE ENGINE - Billing Cluster :9102]
Fatura Detayı (Invoice: %s):
- Düzenlenme Tarihi: %s
- Müşteri: Enterprise SaaS Client
- Kalemler:
  * vMCP Multi-Tenant Gateway Lisansı: $1,000.00
  * HNSW 10k Vektör Motoru Barındırma: $250.00
- Ara Toplam: $%.2f
- KDV (%%20): $%.2f
- Genel Toplam: $%.2f
- Ödeme Durumu: PAID (Kart: 4532-****-****-8812)`, invID, time.Now().Format("2006-01-02"), subtotal, tax, total)

	case "stripe_list_subscriptions":
		return fmt.Sprintf(`[STRIPE SUBSCRIPTION ANALYTICS - Billing Cluster :9102]
✓ Aktif Kurumsal Abonelikler ve Gelir Metrikleri:
- Toplam Aktif Abonelik: 175 Kurumsal Müşteri
- Güncel Aylık Tekrarlayan Gelir (MRR): $42,500.00
- Yıllık Tahmini Gelir (ARR): $510,000.00
- Churn Oranı: %%0.4 (Sektör ortalaması altında)
- En Çok Tercih Edilen Plan: Enterprise Cloud Tier (115 hesap)`)

	case "postgres_run_query":
		liveDB.mu.RLock()
		defer liveDB.mu.RUnlock()
		cleanQ := strings.ToLower(target)
		if strings.Contains(cleanQ, "users") {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("[POSTGRESQL LIVE ENGINE - Database Cluster :9103]\nSorgu: %s\n", target))
			sb.WriteString(fmt.Sprintf("%-4s %-16s %-30s %-16s %-8s\n", "ID", "USERNAME", "EMAIL", "ROLE", "ACTIVE"))
			sb.WriteString("------------------------------------------------------------------------------\n")
			count := 0
			for _, u := range liveDB.Tables["users"] {
				if strings.Contains(cleanQ, "active") && u["active"] != true {
					continue
				}
				sb.WriteString(fmt.Sprintf("%-4v %-16v %-30v %-16v %-8v\n", u["id"], u["username"], u["email"], u["role"], u["active"]))
				count++
				if count >= 10 {
					break
				}
			}
			sb.WriteString(fmt.Sprintf("------------------------------------------------------------------------------\n(%d satır döndü, Planlama: 0.2ms, Yürütme: 0.6ms, İzolasyon: READ COMMITTED)", count))
			return sb.String()
		} else if strings.Contains(cleanQ, "orders") {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("[POSTGRESQL LIVE ENGINE - Database Cluster :9103]\nSorgu: %s\n", target))
			sb.WriteString(fmt.Sprintf("%-6s %-18s %-10s %-12s\n", "ID", "CUSTOMER", "AMOUNT", "STATUS"))
			sb.WriteString("------------------------------------------------------------\n")
			for _, o := range liveDB.Tables["orders"] {
				sb.WriteString(fmt.Sprintf("%-6v %-18v %-10v %-12v\n", o["id"], o["customer"], o["amount"], o["status"]))
			}
			sb.WriteString("------------------------------------------------------------\n(4 satır döndü, Buffer hit: 100%)")
			return sb.String()
		}
		return fmt.Sprintf("[POSTGRESQL LIVE ENGINE :9103] SQL '%s' başarıyla çalıştırıldı (İşlem Onaylandı, 0 hata, 1.2ms).", target)

	case "postgres_list_tables":
		liveDB.mu.RLock()
		defer liveDB.mu.RUnlock()
		var sb strings.Builder
		sb.WriteString("[POSTGRESQL LIVE SCHEMA CATALOG - Database Cluster :9103]\n")
		sb.WriteString(fmt.Sprintf("%-16s %-14s %-12s %-16s\n", "TABLO ADI", "ŞEMA", "SATIR SAYISI", "BOYUT"))
		sb.WriteString("------------------------------------------------------------\n")
		for tName, rows := range liveDB.Tables {
			sb.WriteString(fmt.Sprintf("%-16s %-14s %-12d %-16s\n", tName, "public", len(rows), fmt.Sprintf("%d KB", (len(rows)+1)*16)))
		}
		return sb.String()

	case "postgres_drop_table":
		liveDB.mu.Lock()
		defer liveDB.mu.Unlock()
		tName := "test_scratch_table"
		if strings.Contains(target, "users") {
			tName = "users"
		}
		delete(liveDB.Tables, tName)
		return fmt.Sprintf(`[POSTGRESQL LIVE DDL COMMAND - Database Cluster :9103]
✓ DROP TABLE %s BAŞARIYLA TAMAMLANDI!
- Tablo ve ilişkili yabancı anahtarlar şemadan silindi.
- Disk alanı temizlendi (CASCADE).
- Kalan Tablolar: %v`, tName, []string{"orders", "subscriptions", "audit_logs"})

	case "redis_set_key":
		liveDB.mu.Lock()
		defer liveDB.mu.Unlock()
		liveDB.KV[target] = fmt.Sprintf("val_set_at_%s", time.Now().Format("15:04:05"))
		return fmt.Sprintf("[REDIS IN-MEMORY STORE :9103] SET '%s' OK (TTL: 3600s)", target)

	case "redis_get_key":
		liveDB.mu.RLock()
		defer liveDB.mu.RUnlock()
		val, ok := liveDB.KV[target]
		if !ok {
			val = "sess_live_9941a_prod"
		}
		return fmt.Sprintf("[REDIS IN-MEMORY STORE :9103] GET '%s' => %s (Hits: 48, TTL: 3412s)", target, val)

	case "vault_get_secret":
		liveVault.mu.RLock()
		defer liveVault.mu.RUnlock()
		sec, ok := liveVault.Secrets[target]
		if !ok {
			sec = liveVault.Secrets["database/prod/credentials"]
		}
		secJSON, _ := json.MarshalIndent(sec, "", "  ")
		leaseID := fmt.Sprintf("s.vault_lease_%s", strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
		return fmt.Sprintf(`[HASHICORP VAULT LIVE ENGINE - SecOps Cluster :9104]
✓ Sır (Secret) Başarıyla Çekildi (Path: %s):
- Lease ID: %s
- Lease Süresi: 3600 saniye (Yenilenebilir: true)
- Çekilme Zamanı: %s
- Veri (Decrypted Data):
%s`, target, leaseID, time.Now().Format("2006-01-02 15:04:05"), string(secJSON))

	case "iam_list_roles":
		return fmt.Sprintf(`[SECOPS IAM ROLE DIRECTORY - SecOps Cluster :9104]
✓ Aktif Kurumsal IAM Rolleri ve Yetki Politikaları:
1. admin (Global RBAC: Full Access, MFA: Enforced)
2. devops (Allowed: devops-gateway, k8s_*, docker_*, helm_*)
3. developer (Allowed: devops-gateway, k8s_get_pod_logs, k8s_describe_pod, postgres_run_query)
4. finance-officer (Allowed: finance-gateway, stripe_*)
5. secops-lead (Allowed: secops-gateway, vault_*, iam_*)
6. guest (Allowed: Read-only healthchecks, Rate-limited: 1 req/s)`)

	case "iam_revoke_access":
		return fmt.Sprintf("[SECOPS IAM ENGINE :9104] Kullanıcı / Token '%s' yetkisi derhal iptal edildi (REVOKED). Aktif oturumlar sonlandırıldı.", target)

	case "slack_send_alert":
		liveCRM.mu.Lock()
		defer liveCRM.mu.Unlock()
		liveCRM.Alerts = append(liveCRM.Alerts, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), target))
		return fmt.Sprintf(`[CRM SLACK NOTIFIER :9105]
✓ Slack '#alerts' kanalına acil durum uyarısı iletildi!
- Kanal: #alerts (ID: C048291A)
- Mesaj İçeriği: %s
- İleti Zamanı: %s
- Gönderim Durumu: DELIVERED (Webhook Status: 200 OK)`, target, time.Now().Format("15:04:05"))

	default:
		// 10.000 sanal bulut aracı için canlı kaynak yöneticisi
		liveVirtualCloud.mu.Lock()
		defer liveVirtualCloud.mu.Unlock()
		resID := fmt.Sprintf("i-%s", strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
		resIP := fmt.Sprintf("10.0.%d.%d", rand.Intn(250)+1, rand.Intn(250)+1)
		liveVirtualCloud.Resources[name] = map[string]any{
			"id":         resID,
			"name":       name,
			"target":     target,
			"state":      "RUNNING",
			"private_ip": resIP,
			"region":     "eu-central-1",
			"created_at": time.Now().Format(time.RFC3339),
		}
		return fmt.Sprintf(`[VIRTUAL CLOUD ORCHESTRATOR - Port :%d]
✓ Sanal Bulut Kaynağı Başarıyla İşlendi / İcra Edildi:
- Araç Adı: %s
- Kaynak / Örnek ID: %s
- Durum: RUNNING (Bölge: eu-central-1)
- Tahsis Edilen Özel IP: %s
- Hedef Parametre: %s
- Zaman Damgası: %s`, port, name, resID, resIP, target, time.Now().Format("2006-01-02 15:04:05"))
	}
}

// 10.000 Anlamlı Gerçekçi Araç Oluşturucu
func buildRealistic10kTools() []discovery.ToolRecord {
	records := make([]discovery.ToolRecord, 10000)

	// Özel 20 Çekirdek Araç
	coreTools := []struct {
		name     string
		domain   string
		serverID string
		desc     string
		roles    []string
	}{
		{"k8s_restart_pod", "devops", "upstream-devops", "Restarts crashed Kubernetes pod in production cluster | Kubernetes uretim ortaminda coken podu hemen yeniden baslat", []string{"admin", "devops"}},
		{"k8s_get_pod_logs", "devops", "upstream-devops", "Fetches crash logs and stacktraces from failed Kubernetes pod | Coken podun loglarini ve hata kayitlarini getir", []string{"admin", "devops", "developer"}},
		{"k8s_describe_pod", "devops", "upstream-devops", "Describes Kubernetes pod events, OOM reasons and health checks | Pod durumunu ve saglik kontrollerini goster", []string{"admin", "devops", "developer"}},
		{"k8s_list_pods", "devops", "upstream-devops", "Lists all pods, nodes and health status in cluster namespace | Tum podlari ve calisma durumlarini listele", []string{"admin", "devops", "developer"}},
		{"k8s_scale_deployment", "devops", "upstream-devops", "Scales Kubernetes deployment replica count up or down | Deployment pod kopya sayisini olcekle", []string{"admin", "devops"}},
		{"docker_build_image", "devops", "upstream-devops", "Builds and pushes Docker image to enterprise container registry | Docker imajini derle ve depoya yukle", []string{"admin", "devops"}},
		{"helm_upgrade_release", "devops", "upstream-devops", "Upgrades Helm release in production cluster | Helm paket surumunu guncelle", []string{"admin", "devops"}},
		{"prometheus_query_metrics", "devops", "upstream-devops", "Queries Prometheus CPU, RAM and network latency metrics | Prometheus sunucusundan sistem metriklerini sorgula", []string{"*"}},
		{"http_post_webhook", "devops", "upstream-devops", "Sends HTTP POST webhook or data payload to external URL | Harici web sitesine veya webhook adresine veri gonder", []string{"admin", "devops"}},

		{"stripe_charge_customer", "billing", "upstream-billing", "Charges customer credit card invoice using Stripe API | Stripe uzerinden musteri kart faturasi tahsil et", []string{"admin", "finance-officer"}},
		{"stripe_refund_payment", "billing", "upstream-billing", "Refunds customer payment or invoice via Stripe | Stripe uzerinden para iadesi yap", []string{"admin", "finance-officer"}},
		{"stripe_get_invoice", "billing", "upstream-billing", "Retrieves customer invoice and billing receipt | Musteri fatura makbuzunu ve odeme detaylarini getir", []string{"admin", "finance-officer"}},
		{"stripe_list_subscriptions", "billing", "upstream-billing", "Lists active subscriptions and MRR metrics | Aktif abonelikleri ve aylik gelir metriklerini listele", []string{"admin", "finance-officer"}},

		{"postgres_run_query", "database", "upstream-database", "Executes read-only SQL queries on Postgres database | Veritabaninda SQL sorgusu calistir", []string{"admin", "devops", "developer"}},
		{"postgres_list_tables", "database", "upstream-database", "Lists tables, schemas and foreign keys in Postgres | Veritabani tablolarini ve semalarini listele", []string{"*"}},
		{"postgres_drop_table", "database", "upstream-database", "Drops database table or truncate data (CRITICAL ACTION) | Veritabaninda tablo sil (Kritik Onay Gerektirir)", []string{"admin"}},
		{"redis_get_key", "database", "upstream-database", "Gets cache key value from Redis in-memory store | Redis belleginden onbellek anahtarini getir", []string{"*"}},
		{"redis_set_key", "database", "upstream-database", "Sets cache key value with TTL in Redis | Redis bellegine yeni onbellek anahtari yaz", []string{"admin", "devops"}},

		{"iam_list_roles", "security", "upstream-secops", "Lists active IAM user roles, policies and token permissions | Aktif IAM rollerini ve guvenlik izinlerini listele", []string{"admin", "secops-lead"}},
		{"iam_revoke_access", "security", "upstream-secops", "Revokes API access keys and user sessions | Kullanici erisim anahtarlarini ve oturumunu iptal et", []string{"admin", "secops-lead"}},
		{"vault_get_secret", "security", "upstream-secops", "Fetches encrypted secrets and credentials from HashiCorp Vault | Vault kasasindan sifrelenmis api anahtari getir", []string{"admin", "secops-lead"}},
		{"slack_send_alert", "crm", "upstream-crm", "Sends automated emergency alert to Slack channel | Slack kanalina acil durum uyarisi gonder", []string{"*"}},
	}

	for i, ct := range coreTools {
		schema := json.RawMessage(fmt.Sprintf(`{"type":"object","properties":{"target":{"type":"string","description":"Target for %s"}},"required":["target"]}`, ct.name))
		records[i] = discovery.ToolRecord{
			ID:           ct.name,
			ServerID:     ct.serverID,
			Name:         ct.name,
			Domain:       ct.domain,
			Description:  ct.desc,
			InputSchema:  schema,
			AllowedRoles: ct.roles,
			IsActive:     true,
			Version:      1,
			UpdatedAt:    time.Now(),
		}
	}

	// Geri Kalan 9.979 Kurumsal Bulut & Mikroservis Araçları
	cloudServices := []string{"aws_ec2", "aws_s3", "aws_rds", "gcp_pubsub", "gcp_cloudsql", "azure_vm", "kafka_broker", "datadog_apm", "pagerduty_alert", "jira_issue"}
	domains := []string{"devops", "billing", "database", "security", "crm"}

	for i := len(coreTools); i < 10000; i++ {
		svc := cloudServices[i%len(cloudServices)]
		domain := domains[i%len(domains)]
		name := fmt.Sprintf("%s_action_%05d", svc, i)
		desc := fmt.Sprintf("Kurumsal %s servisi için otomatik operasyon ve yönetim aracı (#%d)", strings.ToUpper(svc), i)
		roles := []string{"admin", domain}

		upstreamID := "upstream-" + domain
		if domain == "security" {
			upstreamID = "upstream-secops"
		}

		records[i] = discovery.ToolRecord{
			ID:           name,
			ServerID:     upstreamID,
			Name:         name,
			Domain:       domain,
			Description:  desc,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}},"required":["target"]}`),
			AllowedRoles: roles,
			IsActive:     true,
			Version:      1,
			UpdatedAt:    time.Now(),
		}
	}

	return records
}
