package toolschema

import (
	"strings"
	"testing"
)

func TestValidationBoundary(t *testing.T) {
	c := Compile([]byte(`{"type":"object","properties":{"count":{"type":"integer","minimum":1}},"required":["count"],"additionalProperties":false}`))
	for _, raw := range []string{`{}`, `{"count":"secret"}`, `{"count":0}`, `{"count":1,"extra":true}`, `null`, `{"count":1} {}`} {
		if c.Validate([]byte(raw)) != ErrArguments {
			t.Fatalf("accepted %s", raw)
		}
	}
	if err := c.Validate([]byte(`{"count":2}`)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"type":"object","$ref":"https://example.com/private"}`, `{"type":"object","$ref":"file:///etc/passwd"}`, `{"type":"object","properties":{"x":{"type":"unknown"}}}`, strings.Repeat("x", 65537)} {
		if Compile([]byte(raw)).Validate([]byte(`{}`)) != ErrSchema {
			t.Fatal("unsafe schema accepted")
		}
	}
}
func TestSnapshotSchemaChanges(t *testing.T) {
	a := Compile([]byte(`{"type":"object"}`))
	b := Compile([]byte(`{"type":"object","required":["value"]}`))
	if a.Validate(nil) != nil || b.Validate(nil) != ErrArguments {
		t.Fatal("schema revision reused")
	}
}

func TestLocalReferencesAndRecursiveBudget(t *testing.T) {
	c := Compile([]byte(`{"type":"object","$defs":{"count":{"type":"integer"}},"properties":{"count":{"$ref":"#/$defs/count"}}}`))
	if c.Validate([]byte(`{"count":1}`)) != nil || c.Validate([]byte(`{"count":"x"}`)) != ErrArguments {
		t.Fatal("local reference validation")
	}
	c = Compile([]byte(`{"type":"object","$defs":{"loop":{"$ref":"#/$defs/loop"}},"properties":{"x":{"$ref":"#/$defs/loop"}}}`))
	if c.Validate([]byte(`{}`)) != ErrSchema {
		t.Fatal("recursive schema was not bounded")
	}
}

func TestLargeIntegerSchemaPreservesPrecision(t *testing.T) {
	c := Compile([]byte(`{"type":"object","properties":{"id":{"const":9007199254740993}},"required":["id"]}`))
	if c.Validate([]byte(`{"id":9007199254740993}`)) != nil || c.Validate([]byte(`{"id":9007199254740992}`)) != ErrArguments {
		t.Fatal("schema numeric precision changed the accepted value")
	}
}
