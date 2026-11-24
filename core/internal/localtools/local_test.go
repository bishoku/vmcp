package localtools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCategoryAndValidation(t *testing.T) {
	d := Definition{Name: "list_components", Parameters: []Parameter{{Name: "category", Type: "string", Choices: []string{"form", "layout"}}}, Response: `{{if or (not .Args.category) (eq .Args.category "form")}}Button{{end}}{{if or (not .Args.category) (eq .Args.category "layout")}}Grid{{end}}`}
	c, err := Compile(Config{[]Definition{d}})
	if err != nil {
		t.Fatal(err)
	}
	for args, want := range map[string]string{`{}`: "ButtonGrid", `{"category":"form"}`: "Button", `{"category":"layout"}`: "Grid"} {
		got, err := c.Render(d.Name, json.RawMessage(args))
		if err != nil || got != want {
			t.Fatalf("%s: got %q, %v", args, got, err)
		}
	}
	for _, args := range []string{`{"category":true}`, `{"category":"unknown"}`, `{"extra":1}`, `null`} {
		if _, err := c.Render(d.Name, json.RawMessage(args)); err == nil {
			t.Errorf("accepted %s", args)
		}
	}
	for _, body := range []string{"{{if}}", "{{template \"a\"}}", "{{range .Args.x}}{{range .Args.x}}{{end}}{{end}}"} {
		d.Response = body
		if _, err := Compile(Config{[]Definition{d}}); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	d.Response = strings.Repeat("{{.Args.category}}", 1024)
	d.Parameters[0].Choices = nil
	c, err = Compile(Config{[]Definition{d}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Render(d.Name, json.RawMessage(`{"category":"`+strings.Repeat("a", 512)+`"}`)); err == nil {
		t.Fatal("unbounded response")
	}
}
