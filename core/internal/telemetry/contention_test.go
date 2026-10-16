package telemetry

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type blockedWriter struct {
	entered, release chan struct{}
	once             sync.Once
}

func (w *blockedWriter) Header() http.Header { return http.Header{} }
func (w *blockedWriter) WriteHeader(int)     {}
func (w *blockedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return len(p), nil
}
func TestSlowScrapeDoesNotBlockRequestsOrOperations(t *testing.T) {
	for _, operationsOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(operationsOnly), func(t *testing.T) {
			r := New()
			w := &blockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
			scrapeDone := make(chan struct{})
			go func() {
				defer close(scrapeDone)
				if operationsOnly {
					r.serveOperations(w)
				} else {
					r.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
				}
			}()
			<-w.entered
			defer func() { close(w.release); <-scrapeDone }()
			completed := make(chan struct{})
			go func() {
				r.BeginOperation(Operation{Family: "virtual_tool_calls", VirtualServer: "bench", Tool: "read"})("ok")
				r.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/mcp/test", nil))
				close(completed)
			}()
			select {
			case <-completed:
			case <-time.After(time.Second):
				t.Fatal("slow metrics writer held the request lock")
			}
		})
	}
}
func BenchmarkOperationsWithScrapes(b *testing.B) {
	r := New()
	for i := 0; i < 2000; i++ {
		r.BeginOperation(Operation{Family: "virtual_tool_calls", VirtualServer: "bench", Tool: fmt.Sprint(i)})("ok")
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
			}
		}
	}()
	defer func() { close(stop); <-done }()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.BeginOperation(Operation{Family: "virtual_tool_calls", VirtualServer: "bench", Tool: "0"})("ok")
		}
	})
}
