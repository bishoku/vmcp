//go:build acceptance

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/restapi"
)

func acceptanceRESTConfig(count int) restapi.Config {
	c := restapi.Config{Auth: restapi.Auth{Type: "none"}, AllowInsecureHTTP: true, TimeoutSeconds: 10}
	for i := 0; i < count; i++ {
		tool := acceptanceTool(i)
		c.Operations = append(c.Operations, restapi.Operation{Name: tool.Name, Description: tool.Description, Enabled: true, Method: "POST", Path: "/" + tool.Name, BodySchema: tool.InputSchema, ResponseSchema: json.RawMessage(`{"type":"object","properties":{"call_id":{"type":"string"},"message":{"type":"string"},"route":{"type":"string"},"tool":{"type":"string"}},"required":["call_id","message","route","tool"],"additionalProperties":false}`)})
	}
	return c
}
func (f *acceptanceFixture) serveREST(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-fixture" {
		http.Error(w, "unauthorized", 401)
		return
	}
	var args struct {
		CallID  string `json:"call_id"`
		Message string `json:"message"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&args) != nil || args.CallID == "" {
		http.Error(w, "invalid body", 400)
		return
	}
	if _, loaded := f.executions.LoadOrStore(args.CallID, true); loaded {
		f.duplicate.Add(1)
	}
	f.count.Add(1)
	select {
	case <-time.After(5 * time.Millisecond):
	case <-r.Context().Done():
		return
	}
	split := strings.LastIndex(r.URL.Path, "/")
	if split < 1 {
		http.Error(w, "invalid route", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"call_id": args.CallID, "message": args.Message, "route": r.URL.Path[:split], "tool": r.URL.Path[split+1:]})
}
