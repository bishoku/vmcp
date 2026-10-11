package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDrainKeepsAdmittedWorkAlive(t *testing.T) {
	gate := &Gate{}
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := gate.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-finish; w.WriteHeader(200) }))
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/call", nil))
	}()
	<-started
	gate.Stop()
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest("POST", "/call", nil))
	if rejected.Code != 503 {
		t.Fatal("admission during drain")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if gate.Wait(ctx) == nil {
		t.Fatal("did not wait for admitted call")
	}
	close(finish)
	<-done
	if err := gate.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestForcedStreamClosureAllowsOutcomeRecording(t *testing.T) {
	gate := &Gate{}
	started, outcome := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(gate.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: started\n\n"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		// Audit completion uses an independent context after HTTP cancellation.
		close(outcome)
	})))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	<-started
	gate.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if srv.Config.Shutdown(ctx) == nil {
		t.Fatal("active stream unexpectedly drained")
	}
	if err := srv.Config.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-outcome:
	case <-time.After(time.Second):
		t.Fatal("cancelled handler could not record outcome")
	}
	if err := gate.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
