// Package omni implements gradual discovery over published virtual servers.
// The search index is never an authorization authority.
package omni

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type Document struct {
	Ref               string `json:"tool_ref"`
	Server            string `json:"server_id"`
	ServerName        string `json:"server_name"`
	Project           string `json:"project"`
	ServerDescription string `json:"server_description"`
	Name              string `json:"name"`
	Description       string `json:"description"`
}

func (d Document) Text() string {
	return d.ServerName + " " + d.ServerDescription + " " + d.Name + " " + d.Description
}

func (d Document) StructuredText() string {
	// Tool purpose comes before shared server context. Bounded fields avoid a
	// verbose server preamble hiding the capability at provider truncation limits.
	return "Tool: " + clipText(d.Name, 128) + "\nPurpose: " + clipText(d.Description, 1800) + "\nService: " + clipText(d.ServerName, 128) + "\nContext: " + clipText(d.ServerDescription, 320)
}
func clipText(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	cut := limit
	for n := limit; n > limit*3/4; n-- {
		if r[n] == ' ' || r[n] == '.' || r[n] == ';' {
			cut = n
			break
		}
	}
	return strings.TrimSpace(string(r[:cut]))
}
func (d Document) Hash() string {
	b, _ := json.Marshal(d)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}

// Ref identifies an exposed tool within a virtual server. Server slugs cannot
// contain '/', while tool names are preserved verbatim, including any slashes.
func Ref(server, name string) string { return server + "/" + name }

func ParseRef(ref string) (string, string, error) {
	server, name, ok := strings.Cut(ref, "/")
	if !ok || server == "" || name == "" {
		return "", "", fmt.Errorf("invalid tool_ref")
	}
	return server, name, nil
}

type Catalog struct {
	Revision  int64      `json:"revision"`
	Documents []Document `json:"documents"`
}
type Access struct {
	Revision int64             `json:"revision"`
	Refs     map[string]string `json:"refs"`
}
type Schema struct {
	Tool    json.RawMessage `json:"tool"`
	Version string          `json:"schema_version"`
}

func SchemaVersion(b []byte) string {
	h := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(h[:])
}
