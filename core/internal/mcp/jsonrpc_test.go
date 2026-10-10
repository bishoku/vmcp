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
		{"object id", `{"jsonrpc":"2.0","id":{},"method":"x"}`, CodeParseError},
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
	if got := NegotiateVersion(ProtocolVersion20250326); got != ProtocolVersion20250326 {
		t.Fatalf("got %s", got)
	}
	if got := NegotiateVersion("1999-01-01"); got != ProtocolVersion20250618 {
		t.Fatalf("unknown version should fall back to newest, got %s", got)
	}
}
