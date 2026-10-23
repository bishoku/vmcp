package telemetry

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestBackgroundMetricsBoundLabelsAndCountConcurrentWork(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); r.Background("discovery", "unchanged") }()
	}
	wg.Wait()
	r.Background("secret-session", "error")
	r.Background("discovery", "secret-error")
	var none *Registry
	none.Background("subscription", "ok")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	text := w.Body.String()
	if !strings.Contains(text, `vmcp_background_operations_total{activity="discovery",outcome="unchanged"} 20`) || strings.Contains(text, "secret") {
		t.Fatal(text)
	}
}
