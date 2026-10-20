package virtual

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIdleSSEDoesNotRetainWriteDeadline(t *testing.T) {
	for _, finalEvent := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean_idle_close", true: "event_after_idle"}[finalEvent], func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if err := writeStreamEvent(w, ": connected\n\n", 50*time.Millisecond); err != nil {
					t.Error(err)
					return
				}
				<-release
				if finalEvent {
					if err := writeStreamEvent(w, "data: done\n\n", time.Second); err != nil {
						t.Error(err)
					}
				}
			}))
			defer server.Close()
			client := &http.Client{Timeout: 3 * time.Second}
			response, err := client.Get(server.URL)
			if err != nil {
				close(release)
				t.Fatal(err)
			}
			defer response.Body.Close()
			reader := bufio.NewReader(response.Body)
			if _, err := reader.ReadString('\n'); err != nil {
				close(release)
				t.Fatal(err)
			}
			// Wait beyond the per-write deadline while the stream is idle.
			time.Sleep(100 * time.Millisecond)
			close(release)
			tail, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("idle SSE closed prematurely: %v", err)
			}
			if finalEvent && string(tail) != "\ndata: done\n\n" {
				t.Fatalf("missing event after idle: %q", tail)
			}
		})
	}
}
