package upstream

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCallerMetaPreservesProgressAndIsolatesRequests(t *testing.T) {
	original := WithRelay(context.Background(), Relay{Meta: json.RawMessage(`{"progressToken":"gateway-progress"}`)})
	one := WithCallerMeta(original, json.RawMessage(`{"bank/context":{"token":"one"}}`))
	two := WithCallerMeta(original, json.RawMessage(`{"bank/context":{"token":"two"}}`))
	for _, tc := range []struct {
		ctx  context.Context
		want string
	}{{one, "one"}, {two, "two"}} {
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(relayFrom(tc.ctx).Meta, &meta); err != nil {
			t.Fatal(err)
		}
		if string(meta["progressToken"]) != `"gateway-progress"` {
			t.Fatal("progress replaced")
		}
		var cc map[string]string
		_ = json.Unmarshal(meta["bank/context"], &cc)
		if cc["token"] != tc.want {
			t.Fatal("cross-call context")
		}
	}
	if string(relayFrom(original).Meta) != `{"progressToken":"gateway-progress"}` {
		t.Fatal("parent context mutated")
	}
}
