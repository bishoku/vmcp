package correlation

import (
	"encoding/json"
	"testing"
)

func TestExtractOptionalBoundedAndIsolated(t *testing.T) {
	c := Config{ConversationID: "/antigravity.google~1conversation_id", RootConversationID: "/antigravity.google~1root_conversation_id", TurnID: "/nested/turn"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"antigravity.google/conversation_id":"child","antigravity.google/root_conversation_id":"root","nested":{"turn":"t1"},"progressToken":"not-a-turn"}`)
	a := c.Extract(raw, "key-1")
	b := c.Extract(raw, "key-2")
	if a.ConversationID != "child" || a.RootConversationID != "root" || a.TurnID != "t1" || a.CorrelationStatus != "captured" || len(a.ConversationGroupID) != 64 || a.ConversationGroupID == b.ConversationGroupID {
		t.Fatalf("extraction: %+v %+v", a, b)
	}
	root := c.Extract(json.RawMessage(`{"antigravity.google/conversation_id":"root","antigravity.google/root_conversation_id":"root"}`), "key-1")
	if root.ConversationGroupID != a.ConversationGroupID || root.TurnID != "" {
		t.Fatal("root and child not grouped")
	}
	if c.Extract(nil, "key-1").CorrelationStatus != "missing" || c.Extract(json.RawMessage(`{"antigravity.google/conversation_id":12}`), "key-1").CorrelationStatus != "invalid" {
		t.Fatal("optional/invalid metadata")
	}
	if (Config{}).Extract(raw, "key-1") != (IDs{}) {
		t.Fatal("disabled mapping")
	}
	for _, p := range []string{"dot.path", "/~2", "/bad~"} {
		if (Config{ConversationID: p}).Validate() == nil {
			t.Fatalf("accepted %s", p)
		}
	}
}
