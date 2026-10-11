package virtual

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

const maxClientRequestBytes = 64 << 10

// Budgets apply to one admitted parent operation, including concurrent requests.
type featureBudget struct {
	mu               sync.Mutex
	requests, tokens int
}

func (b *featureBudget) reserve(tokens int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.requests >= 8 || b.tokens+tokens > 8192 {
		return false
	}
	b.requests++
	b.tokens += tokens
	return true
}
func featureAllowed(method string, vs config.VirtualServer, sess *Session) bool {
	var caps map[string]json.RawMessage
	_ = json.Unmarshal(vs.ClientFeatures.Negotiated(sess.clientCapabilities), &caps)
	switch method {
	case mcp.MethodPing:
		return true
	case "sampling/createMessage":
		return caps["sampling"] != nil
	case "roots/list":
		return caps["roots"] != nil
	case "elicitation/create":
		return caps["elicitation"] != nil
	}
	return false
}
func validateFeatureRequest(message *mcp.Message) (int, error) {
	if len(message.Params) > maxClientRequestBytes {
		return 0, errors.New("client request exceeds limit")
	}
	var p map[string]json.RawMessage
	if len(message.Params) > 0 && (json.Unmarshal(message.Params, &p) != nil || p == nil) {
		return 0, errors.New("invalid request parameters")
	}
	switch message.Method {
	case mcp.MethodPing, "roots/list":
		return 0, nil
	case "sampling/createMessage":
		var tokens int
		var messages []struct {
			Role    string                     `json:"role"`
			Content map[string]json.RawMessage `json:"content"`
		}
		if json.Unmarshal(p["maxTokens"], &tokens) != nil || tokens < 1 || tokens > 4096 || json.Unmarshal(p["messages"], &messages) != nil || len(messages) < 1 || len(messages) > 32 {
			return 0, errors.New("invalid or excessive sampling request")
		}
		for _, m := range messages {
			if (m.Role != "user" && m.Role != "assistant") || m.Content == nil || validateSamplingContent(m.Content) != nil {
				return 0, errors.New("invalid sampling message")
			}
		}
		var include string
		if p["includeContext"] != nil && (json.Unmarshal(p["includeContext"], &include) != nil || (include != "none" && include != "")) {
			return 0, errors.New("cross-server sampling context is not permitted")
		}
		return tokens, nil
	case "elicitation/create":
		if p["mode"] != nil || p["url"] != nil {
			return 0, errors.New("elicitation mode is unsupported for this revision")
		}
		var text string
		if json.Unmarshal(p["message"], &text) != nil || text == "" {
			return 0, errors.New("elicitation message is required")
		}
		var schema struct {
			Type       string                                `json:"type"`
			Properties map[string]map[string]json.RawMessage `json:"properties"`
			Required   []string                              `json:"required"`
		}
		var schemaFields map[string]json.RawMessage
		_ = json.Unmarshal(p["requestedSchema"], &schemaFields)
		for _, key := range []string{"$ref", "$id", "$schema", "allOf", "anyOf", "oneOf"} {
			if schemaFields[key] != nil {
				return 0, errors.New("external or composite elicitation schemas are unsupported")
			}
		}
		if json.Unmarshal(p["requestedSchema"], &schema) != nil || schema.Type != "object" || schema.Properties == nil || len(schema.Properties) > 32 {
			return 0, errors.New("elicitation requires a bounded flat object schema")
		}
		for name, property := range schema.Properties {
			if name == "" || len(name) > 128 {
				return 0, errors.New("invalid elicitation property")
			}
			var typ, format string
			_ = json.Unmarshal(property["type"], &typ)
			_ = json.Unmarshal(property["format"], &format)
			switch typ {
			case "string", "number", "integer", "boolean":
			default:
				return 0, errors.New("elicitation supports primitive properties only")
			}
			if format != "" && format != "email" && format != "uri" && format != "date" && format != "date-time" {
				return 0, errors.New("unsupported elicitation format")
			}
			for _, forbidden := range []string{"$ref", "properties", "items", "allOf", "anyOf", "oneOf"} {
				if property[forbidden] != nil {
					return 0, errors.New("unsupported elicitation schema")
				}
			}
		}
		for _, name := range schema.Required {
			if schema.Properties[name] == nil {
				return 0, errors.New("unknown required elicitation property")
			}
		}
		return 0, nil
	}
	return 0, errors.New("unsupported client request")
}
func validateFeatureResult(method string, response *mcp.Message) error {
	if response.Error != nil {
		return nil
	}
	if len(response.Result) > 1<<20 {
		return errors.New("client response exceeds limit")
	}
	switch method {
	case "roots/list":
		var p struct {
			Roots []struct {
				URI  string `json:"uri"`
				Name string `json:"name"`
			} `json:"roots"`
		}
		if json.Unmarshal(response.Result, &p) != nil || p.Roots == nil || len(p.Roots) > 64 {
			return errors.New("invalid roots response")
		}
		for _, root := range p.Roots {
			u, err := url.Parse(root.URI)
			if err != nil || u.Scheme != "file" || u.Path == "" || len(root.URI) > 4096 {
				return errors.New("invalid client root URI")
			}
		}
	case "sampling/createMessage":
		var p struct {
			Model   string                     `json:"model"`
			Role    string                     `json:"role"`
			Content map[string]json.RawMessage `json:"content"`
		}
		if json.Unmarshal(response.Result, &p) != nil || p.Model == "" || len(p.Model) > 256 || (p.Role != "assistant" && p.Role != "user") || (p.Content == nil || validateSamplingContent(p.Content) != nil) {
			return errors.New("invalid sampling response")
		}
	case "elicitation/create":
		var p struct {
			Action  string         `json:"action"`
			Content map[string]any `json:"content"`
		}
		if json.Unmarshal(response.Result, &p) != nil {
			return errors.New("invalid elicitation response")
		}
		switch p.Action {
		case "decline", "cancel":
			if p.Content != nil {
				return errors.New("declined elicitation must not contain data")
			}
		case "accept":
			if p.Content == nil {
				return errors.New("accepted elicitation requires content")
			}
			for _, v := range p.Content {
				switch v.(type) {
				case string, float64, bool:
				default:
					return errors.New("elicitation content must be primitive")
				}
			}
		default:
			return errors.New("unknown elicitation action")
		}
	}
	return nil
}
func (h *Handler) relayClientRequest(ctx context.Context, message *mcp.Message, vs config.VirtualServer, sess *Session, op delivery.Operation, ids *sync.Map, budget *featureBudget) (*mcp.Message, error) {
	reject := func(code int, text string) (*mcp.Message, error) {
		return mcp.NewErrorResponse(message.ID, code, text), nil
	}
	if !message.IsRequest() {
		return nil, nil
	}
	if !featureAllowed(message.Method, vs, sess) {
		return reject(mcp.CodeMethodNotFound, "client capability not negotiated or permitted")
	}
	tokens, err := validateFeatureRequest(message)
	if err != nil {
		return reject(mcp.CodeInvalidParams, err.Error())
	}
	if !budget.reserve(tokens) {
		return reject(-32000, "client request budget exceeded")
	}
	if err := checkAuthorization(ctx); err != nil {
		return reject(-32000, "authorization changed")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	mapped := delivery.UUID()
	ids.Store(message.ID.String(), mapped)
	defer ids.Delete(message.ID.String())
	forwarded := *message
	forwarded.ID = mcp.NewStringID(mapped)
	if message.Method == mcp.MethodPing || message.Method == "roots/list" {
		forwarded.Params = nil
	} else if vs.PII != nil {
		forwarded.Params, err = inspectCatalogResult(forwarded.Params, vs)
		if err != nil {
			return reject(mcp.CodeInvalidParams, "request content cannot be inspected")
		}
	}
	if err := h.Delivery.Request(requestCtx, op, mapped); err != nil {
		return nil, err
	}

	if message.Method != "ping" && message.Method != "roots/list" {
		var p map[string]json.RawMessage
		_ = json.Unmarshal(forwarded.Params, &p)
		if p == nil {
			p = map[string]json.RawMessage{}
		}
		var metadata map[string]json.RawMessage
		_ = json.Unmarshal(p["_meta"], &metadata)
		if metadata == nil {
			metadata = map[string]json.RawMessage{}
		}
		metadata["vmcp/upstream"], _ = json.Marshal(upstream.Origin(ctx))
		p["_meta"], _ = json.Marshal(metadata)
		forwarded.Params, _ = json.Marshal(p)
	}
	raw, _ := json.Marshal(forwarded)
	if err := h.auditDelivery(requestCtx, sess, op.ID, "mcp.client_request"); err != nil {
		return nil, err
	}
	if err := h.Delivery.Append(requestCtx, op, raw, false); err != nil {
		return nil, err
	}
	reply, err := h.Delivery.WaitReply(requestCtx, op, mapped)
	if err != nil {
		return reject(-32800, "client request expired or cancelled")
	}
	response, err := mcp.Decode(reply)
	if err != nil {
		return reject(mcp.CodeInvalidParams, "invalid client response")
	}
	if err := checkAuthorization(ctx); err != nil {
		return reject(-32000, "authorization changed")
	}
	if response.Error == nil {
		masked, inspectErr := inspectCatalogResult(response.Result, vs)
		if inspectErr != nil {
			return reject(mcp.CodeInvalidParams, "client response cannot be inspected")
		}
		response.Result = masked
	}
	if err := validateFeatureResult(message.Method, response); err != nil {
		return reject(mcp.CodeInvalidParams, err.Error())
	}
	if message.Method == "elicitation/create" {
		if err := validateElicitationContent(forwarded.Params, response); err != nil {
			return reject(mcp.CodeInvalidParams, err.Error())
		}
	}
	response.ID = message.ID
	return response, nil
}

var logLevels = map[string]int{"debug": 0, "info": 1, "notice": 2, "warning": 3, "error": 4, "critical": 5, "alert": 6, "emergency": 7}

func (h *Handler) relayLog(ctx context.Context, message *mcp.Message, sess *Session, op delivery.Operation) error {
	var p struct {
		Level string `json:"level"`
	}
	if json.Unmarshal(message.Params, &p) != nil {
		return errors.New("invalid logging notification")
	}
	level, ok := logLevels[p.Level]
	if !ok {
		return errors.New("invalid logging severity")
	}
	var threshold string
	if err := h.Delivery.Pool.QueryRow(ctx, `SELECT logging_level FROM sessions WHERE id=$1 AND terminated_at IS NULL`, sess.id).Scan(&threshold); err != nil {
		return err
	}
	if level < logLevels[threshold] {
		return nil
	}
	safe, _ := mcp.NewRequest(mcp.ID{}, "notifications/message", map[string]string{"level": p.Level, "logger": "upstream", "data": "upstream log payload omitted by gateway policy"})
	raw, _ := json.Marshal(safe)
	if err := h.auditDelivery(ctx, sess, op.ID, "mcp.logging_notification"); err != nil {
		return err
	}
	return h.Delivery.Append(ctx, op, raw, false)
}

// Validate the host's accepted form against the same bounded schema it saw.
// Decline/cancel carries no content and is forwarded without inventing consent.
func validateElicitationContent(request json.RawMessage, response *mcp.Message) error {
	if response.Error != nil {
		return nil
	}
	var result struct {
		Action  string         `json:"action"`
		Content map[string]any `json:"content"`
	}
	if json.Unmarshal(response.Result, &result) != nil {
		return errors.New("invalid elicitation response")
	}
	if result.Action != "accept" {
		return nil
	}
	var p struct {
		Schema struct {
			Properties map[string]map[string]json.RawMessage `json:"properties"`
			Required   []string                              `json:"required"`
		} `json:"requestedSchema"`
	}
	if json.Unmarshal(request, &p) != nil {
		return errors.New("invalid requested schema")
	}
	for _, name := range p.Schema.Required {
		if _, ok := result.Content[name]; !ok {
			return errors.New("required elicitation property missing")
		}
	}
	for name, value := range result.Content {
		property, ok := p.Schema.Properties[name]
		if !ok {
			return errors.New("unknown elicitation property")
		}
		var typ string
		_ = json.Unmarshal(property["type"], &typ)
		switch typ {
		case "string":
			text, ok := value.(string)
			if !ok {
				return errors.New("elicitation property has wrong type")
			}
			for _, limit := range []string{"minLength", "maxLength"} {
				var n int
				if raw := property[limit]; raw != nil {
					if json.Unmarshal(raw, &n) != nil || (limit == "minLength" && len([]rune(text)) < n) || (limit == "maxLength" && len([]rune(text)) > n) {
						return errors.New("elicitation string violates bounds")
					}
				}
			}
			var choices []string
			if raw := property["enum"]; raw != nil {
				if json.Unmarshal(raw, &choices) != nil {
					return errors.New("invalid elicitation enum")
				}
				found := false
				for _, choice := range choices {
					found = found || choice == text
				}
				if !found {
					return errors.New("elicitation value not in enum")
				}
			}
		case "number", "integer":
			number, ok := value.(float64)
			if !ok || (typ == "integer" && number != math.Trunc(number)) {
				return errors.New("elicitation property has wrong type")
			}
			for _, limit := range []string{"minimum", "maximum"} {
				var n float64
				if raw := property[limit]; raw != nil {
					if json.Unmarshal(raw, &n) != nil || (limit == "minimum" && number < n) || (limit == "maximum" && number > n) {
						return errors.New("elicitation number violates bounds")
					}
				}
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return errors.New("elicitation property has wrong type")
			}
		default:
			return errors.New("unsupported elicitation property")
		}
	}
	return nil
}

func (h *Handler) configureLogging(ctx context.Context, sess *Session, c upstream.Conn) error {
	if h.Delivery == nil {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var level string
	if err := h.Delivery.Pool.QueryRow(bounded, `SELECT logging_level FROM sessions WHERE id=$1 AND terminated_at IS NULL`, sess.id).Scan(&level); err != nil {
		return err
	}
	return upstream.SetLoggingLevel(bounded, c, level)
}

func validateSamplingContent(content map[string]json.RawMessage) error {
	var typ string
	if json.Unmarshal(content["type"], &typ) != nil {
		return errors.New("sampling content type required")
	}
	switch typ {
	case "text":
		var text string
		if json.Unmarshal(content["text"], &text) != nil {
			return errors.New("sampling text required")
		}
	case "image", "audio":
		var data, mime string
		if json.Unmarshal(content["data"], &data) != nil || json.Unmarshal(content["mimeType"], &mime) != nil || mime == "" {
			return errors.New("sampling media fields required")
		}
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			return errors.New("sampling media must be base64")
		}
	default:
		return errors.New("unsupported sampling content type")
	}
	return nil
}
