package mcp

import "encoding/json"

// Tool extension fields survive discovery without overriding gateway routing.
func (t *Tool) UnmarshalJSON(raw []byte) error {
	type plain Tool
	var value plain
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(raw, &extra); err != nil {
		return err
	}
	for _, key := range []string{"name", "title", "description", "inputSchema", "outputSchema", "annotations", "_meta"} {
		delete(extra, key)
	}
	value.Extra = extra
	*t = Tool(value)
	return nil
}
func (t Tool) MarshalJSON() ([]byte, error) {
	type plain Tool
	raw, err := json.Marshal(plain(t))
	if err != nil {
		return nil, err
	}
	var out map[string]json.RawMessage
	_ = json.Unmarshal(raw, &out)
	for key, value := range t.Extra {
		switch key {
		case "name", "title", "description", "inputSchema", "outputSchema", "annotations", "_meta":
			continue
		}
		if _, known := out[key]; !known {
			out[key] = value
		}
	}
	return json.Marshal(out)
}
func MergeToolMetadata(base Tool, definition []byte) Tool {
	var rich Tool
	if len(definition) <= 2 || json.Unmarshal(definition, &rich) != nil {
		return base
	}
	rich.Name, rich.Description, rich.InputSchema, rich.Annotations = base.Name, base.Description, base.InputSchema, base.Annotations
	return rich
}
