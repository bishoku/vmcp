package mockupstream_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/mockupstream"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

// The frontend tests generate exactly these matchers. Run them against real
// WireMock through the REST executor, including MCP's body argument wrapper.
func TestVisualEditorMatchersAgainstWireMock(t *testing.T) {
	base := os.Getenv("VMCP_TEST_WIREMOCK_URL")
	if base == "" {
		if os.Getenv("VMCP_REQUIRE_INTEGRATION") == "1" {
			t.Fatal("VMCP_TEST_WIREMOCK_URL required")
		}
		t.Skip("VMCP_TEST_WIREMOCK_URL not set")
	}
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	data, err := os.ReadFile("../../../web/src/connections/mock-visual-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name    string
		Request map[string]json.RawMessage
		Cases   []struct {
			Body    json.RawMessage
			Matched bool
		}
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	owner := mockupstream.Owner(fmt.Sprintf("visual-editor-%d", time.Now().UnixNano()))
	client := mockupstream.NewClient(base)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Reconcile(ctx, owner, nil); err != nil {
			t.Error(err)
		}
	})
	cfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, TimeoutSeconds: 3, AllowInsecureHTTP: true, Mock: &mockupstream.Config{Namespace: mockupstream.Namespace(owner, "visual")}}
	for _, fixture := range fixtures {
		cfg.Operations = append(cfg.Operations, restapi.Operation{Name: fixture.Name, Method: "POST", Path: "/" + fixture.Name, Enabled: true, BodySchema: raw(`{"type":"object"}`), MockRules: []mockupstream.Rule{
			{Name: "match", Priority: 1, Request: fixture.Request, Response: map[string]json.RawMessage{"status": raw(`200`), "jsonBody": raw(`{"matched":true}`)}},
			{Name: "fallback", Priority: 2, Response: map[string]json.RawMessage{"status": raw(`200`), "jsonBody": raw(`{"matched":false}`)}},
		}})
	}
	compiled, err := restapi.Compile(base, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Reconcile(ctx, owner, mockupstream.Mappings(owner, *cfg.Mock, cfg.MockOperations())); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		for i, tc := range fixture.Cases {
			t.Run(fmt.Sprintf("%s/%d", fixture.Name, i), func(t *testing.T) {
				args := raw(`{"body":` + string(tc.Body) + `}`)
				result, err := compiled.Execute(ctx, base, fixture.Name, args, nil, tlsconf.Settings{}, restapi.Identity{}, nil)
				if err != nil || !strings.Contains(string(result), fmt.Sprintf(`"matched":%t`, tc.Matched)) {
					t.Fatalf("matched=%t: %s (%v)", tc.Matched, result, err)
				}
			})
		}
	}
}
