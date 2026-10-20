package virtual

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/bishoku/vmcp/core/internal/config"
)

func localCatalogResult(object config.VirtualCatalogObject, method string, params map[string]json.RawMessage) (json.RawMessage, error) {
	if method == "resources/read" {
		return json.Marshal(map[string]any{"contents": []any{map[string]any{"uri": object.Original, "mimeType": object.Local.MimeType, "text": object.Local.Text}}})
	}
	if method != "prompts/get" {
		return nil, errors.New("local operation unavailable")
	}
	args := map[string]string{}
	var defs []config.PromptArgument
	if err := json.Unmarshal(object.Definition["arguments"], &defs); err != nil {
		return nil, err
	}
	for _, d := range defs {
		args[d.Name] = ""
	}
	if len(params["arguments"]) > 0 {
		var supplied map[string]string
		if err := json.Unmarshal(params["arguments"], &supplied); err != nil {
			return nil, err
		}
		for k, v := range supplied {
			args[k] = v
		}
	}
	messages := []any{}
	total := 0
	for _, message := range object.Local.Messages {
		text, err := config.RenderGuidance(message.Text, object.Local.ToolReferences, args, false)
		if err != nil {
			return nil, err
		}
		total += utf8.RuneCountInString(text)
		if total > 100000 {
			return nil, errors.New("rendered prompt too large")
		}
		messages = append(messages, map[string]any{"role": message.Role, "content": map[string]string{"type": "text", "text": text}})
	}
	return json.Marshal(map[string]any{"description": object.Definition.String("description"), "messages": messages})
}
