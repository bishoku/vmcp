package mcp

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDecodeKinds(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		check func(*Message) bool
	}{
		{"request int id", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, (*Message).IsRequest},
		{"request string id", `{"jsonrpc":"2.0","id":"abc","method":"ping"}`, (*Message).IsRequest},
		{"notification", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, (*Message).IsNotification},
		{"result", `{"jsonrpc":"2.0","id":1,"result":{}}`, (*Message).IsResponse},
		{"error", `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"nope"}}`, (*Message).IsResponse},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := Decode([]byte(c.in))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !c.check(m) {
				t.Fatalf("wrong message kind: %+v", m)
			}
		})
	}
}

func TestDecodeRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		code int
	}{
		{"garbage", `{not json`, CodeParseError},
		{"wrong version", `{"jsonrpc":"1.0","id":1,"method":"x"}`, CodeInvalidRequest},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"x"}]`, CodeInvalidRequest},
		{"empty envelope", `{"jsonrpc":"2.0"}`, CodeInvalidRequest},
		{"object id", `{"jsonrpc":"2.0","id":{},"method":"x"}`, CodeInvalidRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Decode([]byte(c.in))
			var e *Error
			if !errors.As(err, &e) || e.Code != c.code {
				t.Fatalf("want code %d, got %v", c.code, err)
			}
		})
	}
}

func TestIDRoundTripVerbatim(t *testing.T) {
	for _, id := range []string{`42`, `"abc"`, `"1"`} {
		in := `{"jsonrpc":"2.0","id":` + id + `,"method":"ping"}`
		m, err := Decode([]byte(in))
		if err != nil {
			t.Fatal(err)
		}
		resp, _ := NewResult(m.ID, map[string]any{})
		out, _ := json.Marshal(resp)
		var back struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.Unmarshal(out, &back)
		if string(back.ID) != id {
			t.Fatalf("id changed: want %s got %s", id, back.ID)
		}
	}
}

func TestNotificationOmitsID(t *testing.T) {
	m := &Message{JSONRPC: JSONRPCVersion, Method: MethodInitialized}
	b, _ := json.Marshal(m)
	if string(b) != `{"jsonrpc":"2.0","method":"notifications/initialized"}` {
		t.Fatalf("unexpected encoding: %s", b)
	}
}

func TestNegotiateVersion(t *testing.T) {
	if got := NegotiateVersion(ProtocolVersion20250326); got != ProtocolVersion20250618 {
		t.Fatalf("got %s", got)
	}
	if got := NegotiateVersion("1999-01-01"); got != ProtocolVersion20250618 {
		t.Fatalf("unknown version should fall back to newest, got %s", got)
	}
}

// Protocol matrix: syntactically valid JSON with an invalid MCP envelope is
// InvalidRequest; only malformed JSON is ParseError.
func TestProtocolEnvelopeMatrix(t *testing.T) {
	for _, input := range []string{
		`null`, `1`, `[]`,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1.5,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":true,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1,"method":""}`,
		`{"jsonrpc":"2.0","id":1,"method":"ping","result":{}}`,
		`{"jsonrpc":"2.0","method":"ping","error":null}`,
		`{"jsonrpc":"2.0","id":1,"method":"ping","params":[]}`,
		`{"jsonrpc":"2.0","id":1,"method":"ping","params":null}`,
		`{"jsonrpc":"2.0","result":{}}`,
		`{"jsonrpc":"2.0","id":null,"result":{}}`,
		`{"jsonrpc":"2.0","id":1,"result":{},"error":null}`,
		`{"jsonrpc":"2.0","id":1,"result":null}`,
		`{"jsonrpc":"2.0","id":1,"result":{},"params":{}}`,
		`{"jsonrpc":"2.0","id":1,"error":null}`,
		`{"jsonrpc":"2.0","id":1,"error":{"message":"x"}}`,
		`{"jsonrpc":"2.0","id":1,"error":{"code":null,"message":"x"}}`,
		`{"jsonrpc":"2.0","id":1,"error":{"code":1}}`,
		`{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":null}}`,
	} {
		t.Run(input, func(t *testing.T) {
			_, err := Decode([]byte(input))
			var rpc *Error
			if !errors.As(err, &rpc) || rpc.Code != CodeInvalidRequest {
				t.Fatalf("want InvalidRequest, got %v", err)
			}
		})
	}
}

func TestInitializeContractMatrix(t *testing.T) {
	for _, raw := range []string{
		`{}`, `null`,
		`{"protocolVersion":"2025-06-18","clientInfo":{"name":"c","version":"1"}}`,
		`{"protocolVersion":"2025-06-18","capabilities":null,"clientInfo":{"name":"c","version":"1"}}`,
		`{"protocolVersion":"2025-06-18","capabilities":[],"clientInfo":{"name":"c","version":"1"}}`,
		`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":null}`,
		`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c"}}`,
		`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":null,"version":"1"}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ValidateInitializeParams(json.RawMessage(raw)); err == nil {
				t.Fatal("accepted incomplete initialize")
			}
		})
	}
	raw := json.RawMessage(`{"protocolVersion":"unknown","capabilities":{"experimental":{"future":{}}},"clientInfo":{"name":"c","version":"1","title":"extension"}}`)
	p, err := ValidateInitializeParams(raw)
	if err != nil || NegotiateVersion(p.ProtocolVersion) != ProtocolVersion20250618 {
		t.Fatalf("valid fallback: %v", err)
	}
	if SupportsVersion(ProtocolVersion20250326) || SupportsVersion("2024-11-05") {
		t.Fatal("advertised unimplemented revision")
	}
}

func TestDecodeRejectsInvalidUTF8(t *testing.T) {
	raw := append([]byte(`{"jsonrpc":"2.0","method":"`), 0xff)
	raw = append(raw, []byte(`"}`)...)
	_, err := Decode(raw)
	var rpc *Error
	if !errors.As(err, &rpc) || rpc.Code != CodeParseError {
		t.Fatalf("invalid UTF-8 accepted: %v", err)
	}
}
