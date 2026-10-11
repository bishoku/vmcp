package config

import "encoding/json"

// ClientFeatures is an explicit operator permission. Client negotiation is also
// mandatory; none of these flags supplies gateway filesystem/model credentials.
type ClientFeatures struct {
	Sampling    bool `json:"sampling"`
	Roots       bool `json:"roots"`
	Elicitation bool `json:"elicitation"`
}

func (p ClientFeatures) Negotiated(raw json.RawMessage) json.RawMessage {
	var offered map[string]json.RawMessage
	_ = json.Unmarshal(raw, &offered)
	allowed := map[string]any{}
	for _, entry := range []struct {
		name    string
		allowed bool
	}{{"sampling", p.Sampling}, {"roots", p.Roots}, {"elicitation", p.Elicitation}} {
		var capability map[string]json.RawMessage
		if !entry.allowed || json.Unmarshal(offered[entry.name], &capability) != nil || capability == nil {
			continue
		}
		if entry.name == "roots" {
			var changed bool
			_ = json.Unmarshal(capability["listChanged"], &changed)
			allowed[entry.name] = map[string]bool{"listChanged": changed}
		} else {
			allowed[entry.name] = struct{}{}
		}
	}
	out, _ := json.Marshal(allowed)
	return out
}
