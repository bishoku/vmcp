package fixture

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// NewLegacySSE returns a fixture speaking the legacy HTTP+SSE transport (2024-11-05):
// GET opens an event stream that announces a POST endpoint; replies arrive on the stream.
// It must be mounted so that the POST endpoint ("?sid=...", relative to the GET URL) resolves to it.
func NewLegacySSE(opts Options) http.Handler {
	return &legacy{inner: &server{opts: opts, sessions: map[string]bool{}}, streams: map[string]chan []byte{}}
}

type legacy struct {
	inner *server
	mu    sync.Mutex
	n     int
	// streams maps session id to the outbound event queue.
	streams map[string]chan []byte
}

func (l *legacy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if l.inner.opts.Token != "" && r.Header.Get("Authorization") != "Bearer "+l.inner.opts.Token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		l.stream(w, r)
	case http.MethodPost:
		l.post(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (l *legacy) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	l.mu.Lock()
	l.n++
	sid := fmt.Sprintf("legacy-%d", l.n)
	q := make(chan []byte, 16)
	l.streams[sid] = q
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.streams, sid)
		l.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = fmt.Fprintf(w, "event: endpoint\ndata: ?sid=%s\n\n", sid)
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-q:
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			fl.Flush()
		}
	}
}

func (l *legacy) post(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	q := l.streams[r.URL.Query().Get("sid")]
	l.mu.Unlock()
	if q == nil {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}
	m, err := mcp.Decode(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	if !m.IsRequest() {
		return
	}
	var resp *mcp.Message
	switch m.Method {
	case mcp.MethodInitialize:
		resp, _ = mcp.NewResult(m.ID, mcp.InitializeResult{
			ProtocolVersion: mcp.ProtocolVersion20250618,
			Capabilities:    mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}},
			ServerInfo:      mcp.Implementation{Name: "vmcp-fixture-legacy", Version: "0.1.0"},
		})
	case mcp.MethodPing:
		resp, _ = mcp.NewResult(m.ID, struct{}{})
	case mcp.MethodToolsList:
		resp, _ = mcp.NewResult(m.ID, mcp.ListToolsResult{Tools: Tools()})
	case mcp.MethodToolsCall:
		resp = l.inner.call(m, r)
	default:
		resp = mcp.NewErrorResponse(m.ID, mcp.CodeMethodNotFound, "method not found: "+m.Method)
	}
	out, _ := json.Marshal(resp)
	q <- out
}
