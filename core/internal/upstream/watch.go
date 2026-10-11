package upstream

import (
	"context"
	"errors"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"mime"
	"net/http"
)

var ErrSubscriptionTransport = errors.New("physical subscription transport unavailable")

// Watch receives unsolicited notifications on the initialized physical session.
// Legacy SSE already has its read loop; Streamable HTTP opens its optional GET.
func Watch(ctx context.Context, c Conn) error {
	switch client := c.(type) {
	case *SSEClient:
		<-ctx.Done()
		return ctx.Err()
	case *Client:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "text/event-stream")
		client.applyHeaders(req, "")
		resp, err := client.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if resp.StatusCode != 200 || mt != "text/event-stream" {
			return ErrSubscriptionTransport
		}
		_, err = client.readSSEContext(ctx, resp.Body, mcp.ID{})
		return err
	default:
		return errors.New("physical subscription transport unavailable")
	}
}

// SetNotificationHandler must be called before Initialize, avoiding mutation of
// callbacks while either transport's read loop is active.
func SetNotificationHandler(c Conn, handler func(*mcp.Message)) {
	switch client := c.(type) {
	case *Client:
		client.OnServerMessage = handler
	case *SSEClient:
		client.OnServerMessage = handler
	}
}
