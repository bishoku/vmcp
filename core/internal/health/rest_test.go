package health

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type restSnapshot struct{ snapshot *config.Snapshot }

func (s restSnapshot) Snapshot() *config.Snapshot { return s.snapshot }
func TestRESTIsNeverAutomaticallyProbed(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	up := config.Upstream{Kind: "rest", URL: server.URL, REST: &restapi.Compiled{}}
	checker := Checker{Snap: restSnapshot{&config.Snapshot{Upstreams: map[string]config.Upstream{"rest": up}}}}
	checker.CheckOnce(context.Background())
	if _, _, err := Probe(context.Background(), up); err == nil {
		t.Fatal("REST probe allowed")
	}
	if calls.Load() != 0 {
		t.Fatal("REST service contacted by health check")
	}
}
