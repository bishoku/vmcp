package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"time"
)

type Relay struct {
	Meta        json.RawMessage
	Message     func(context.Context, *mcp.Message) (*mcp.Message, error)
	OwnsRequest func(mcp.ID) bool
}
type relayKey struct{}

func WithRelay(ctx context.Context, relay Relay) context.Context {
	return context.WithValue(ctx, relayKey{}, relay)
}
func relayFrom(ctx context.Context) Relay { relay, _ := ctx.Value(relayKey{}).(Relay); return relay }
func handleMessage(ctx context.Context, msg *mcp.Message, send func(context.Context, *mcp.Message) error, fallback func(*mcp.Message)) error {
	relay := relayFrom(ctx)
	if relay.Message != nil {
		response, err := relay.Message(ctx, msg)
		if err != nil {
			return err
		}
		if response != nil {
			return send(ctx, response)
		}
		return nil
	}
	if fallback != nil {
		fallback(msg)
	}
	if msg.IsRequest() && msg.Method == mcp.MethodPing {
		response, _ := mcp.NewResult(msg.ID, struct{}{})
		return send(ctx, response)
	}
	if msg.IsRequest() {
		return send(ctx, mcp.NewErrorResponse(msg.ID, mcp.CodeMethodNotFound, "client capability is not available"))
	}
	return nil
}
func (c *Client) Send(ctx context.Context, msg *mcp.Message) error {
	_, err := c.send(ctx, msg)
	return err
}
func (c *SSEClient) Send(ctx context.Context, msg *mcp.Message) error { return c.post(ctx, msg) }

func cancelRequest(ctx context.Context, method string, id mcp.ID, send func(context.Context, *mcp.Message) error) {
	if ctx.Err() == nil || method == mcp.MethodInitialize {
		return
	}
	cancelCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	notification, _ := mcp.NewRequest(mcp.ID{}, mcp.MethodCancelled, map[string]any{"requestId": id})
	_ = send(cancelCtx, notification)
}

func selectRelay(contexts map[string]context.Context, message *mcp.Message) context.Context {
	for _, ctx := range contexts {
		relay := relayFrom(ctx)
		if ctx.Err() != nil || relay.Message == nil {
			continue
		}
		if message.Method == mcp.MethodProgress {
			var params, meta map[string]json.RawMessage
			_ = json.Unmarshal(message.Params, &params)
			_ = json.Unmarshal(relay.Meta, &meta)
			if len(meta["progressToken"]) == 0 || !bytes.Equal(params["progressToken"], meta["progressToken"]) {
				continue
			}
		}
		if message.Method == mcp.MethodCancelled {
			var params struct {
				RequestID mcp.ID `json:"requestId"`
			}
			_ = json.Unmarshal(message.Params, &params)
			if relay.OwnsRequest == nil || !relay.OwnsRequest(params.RequestID) {
				continue
			}
		}
		return ctx
	}
	return context.Background()
}
