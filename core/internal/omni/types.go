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
func (d Document) Hash() string {
	b, _ := json.Marshal(d)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}
func Ref(server, name string) string {
	b, _ := json.Marshal([]string{server, name})
	return base64.RawURLEncoding.EncodeToString(b)
}
func ParseRef(ref string) (string, string, error) {
	b, e := base64.RawURLEncoding.DecodeString(ref)
	var parts []string
	if e != nil || json.Unmarshal(b, &parts) != nil || len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[0], "/") {
		return "", "", fmt.Errorf("invalid tool_ref")
	}
	return parts[0], parts[1], nil
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

func SchemaVersion(b []byte) string { h := sha256.Sum256(b); return fmt.Sprintf("%x", h) }
