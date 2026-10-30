// Package correlation extracts optional caller-supplied audit IDs. These are not identity or authority.
package correlation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode"
)

type Config struct {
	ConversationID     string `json:"conversationId,omitempty"`
	RootConversationID string `json:"rootConversationId,omitempty"`
	TurnID             string `json:"turnId,omitempty"`
}

func (c Config) Enabled() bool { return c != (Config{}) }
func (c Config) Validate() error {
	for _, p := range []string{c.ConversationID, c.RootConversationID, c.TurnID} {
		if p == "" {
			continue
		}
		if len(p) > 512 || !strings.HasPrefix(p, "/") || len(strings.Split(p, "/")) > 17 {
			return errors.New("use JSON Pointers rooted at params._meta, maximum 512 bytes and 16 segments")
		}
		for i := 0; i < len(p); i++ {
			if p[i] == '~' {
				if i+1 >= len(p) || (p[i+1] != '0' && p[i+1] != '1') {
					return errors.New("invalid JSON Pointer escape; use ~1 for / and ~0 for ~")
				}
				i++
			}
		}
	}
	return nil
}

type IDs struct {
	ConversationID      string `json:"conversationId,omitempty"`
	RootConversationID  string `json:"rootConversationId,omitempty"`
	TurnID              string `json:"turnId,omitempty"`
	ConversationGroupID string `json:"conversationGroupId,omitempty"`
	CorrelationStatus   string `json:"correlationStatus,omitempty"`
}

func (c Config) Extract(raw json.RawMessage, credentialID string) IDs {
	if !c.Enabled() || credentialID == "" {
		return IDs{}
	}
	out := IDs{CorrelationStatus: "missing"}
	if len(raw) > 32768 {
		out.CorrelationStatus = "invalid"
		return out
	}
	if len(raw) == 0 {
		return out
	}
	var meta any
	if json.Unmarshal(raw, &meta) != nil {
		out.CorrelationStatus = "invalid"
		return out
	}
	invalid := false
	read := func(p string) string {
		if p == "" {
			return ""
		}
		var v any = meta
		for _, part := range strings.Split(p[1:], "/") {
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			switch current := v.(type) {
			case map[string]any:
				v = current[part]
			case []any:
				index, err := strconv.Atoi(part)
				if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != part {
					return ""
				}
				v = current[index]
			default:
				return ""
			}
		}
		if v == nil {
			return ""
		}
		s, ok := v.(string)
		if !ok || len(s) > 256 || strings.TrimSpace(s) == "" || strings.IndexFunc(s, unicode.IsControl) >= 0 {
			invalid = true
			return ""
		}
		return s
	}
	out.ConversationID = read(c.ConversationID)
	out.RootConversationID = read(c.RootConversationID)
	out.TurnID = read(c.TurnID)
	root := out.RootConversationID
	if root == "" {
		root = out.ConversationID
	}
	if root != "" {
		b, _ := json.Marshal([]string{"vmcp.conversation.v1", credentialID, root})
		sum := sha256.Sum256(b)
		out.ConversationGroupID = hex.EncodeToString(sum[:])
		out.CorrelationStatus = "captured"
	}
	if invalid {
		out.CorrelationStatus = "invalid"
	}
	return out
}
