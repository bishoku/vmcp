package lifecycle

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkerPanicDrainsWithoutLeakingValue(t *testing.T) {
	var logs bytes.Buffer
	gate := &Gate{}
	s := NewSupervisor(slog.New(slog.NewJSONHandler(&logs, nil)), gate.Stop)
	gate.Supervisor = s
	ctx := WithSupervisor(context.Background(), s)
	go func() { defer Guard(ctx, "fixture"); panic("SECRET credential") }()
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("panic not reported")
	}
	if !gate.Draining() || s.Err() == nil {
		t.Fatal("not failed closed")
	}
	if strings.Contains(logs.String(), "SECRET") || !strings.Contains(logs.String(), "fixture") {
		t.Fatal(logs.String())
	}
	w := httptest.NewRecorder()
	gate.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 {
		t.Fatal("ready after failure")
	}
}
