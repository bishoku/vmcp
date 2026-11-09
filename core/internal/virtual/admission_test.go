package virtual

import (
	"github.com/bishoku/vmcp/core/internal/admission"
	"sync"
	"testing"
)

func TestAdmissionLiveLimitsAndDetachedWorker(t *testing.T) {
	h := &Handler{}
	owner := Owner{Mode: "agent-key", Principal: "key"}
	cfg := admission.Settings{Total: 4, PerServer: 3, PerPrincipal: 2}
	release, ok := h.acquire(owner, "server", "", cfg)
	if !ok {
		t.Fatal("admission")
	}
	slot := newAdmissionSlot(release)
	releaseHTTP := slot.releaseOnce()
	releaseWorker := slot.retain()
	releaseHTTP()
	releaseHTTP()
	lowered := cfg
	lowered.PerPrincipal = 1
	if _, ok := h.acquire(owner, "server", "", lowered); ok {
		t.Fatal("detached worker lost protection")
	}
	other := &Handler{}
	local, ok := other.acquire(owner, "server", "", lowered)
	if !ok {
		t.Fatal("limits must be per pod")
	}
	local()
	raised := cfg
	raised.PerPrincipal = 3
	second, ok := h.acquire(owner, "server", "", raised)
	if !ok {
		t.Fatal("raised limit ignored")
	}
	second()
	second()
	releaseWorker()
	releaseWorker()
	if len(h.budgets) != 0 {
		t.Fatal("reservation leaked", h.budgets)
	}
	next, ok := h.acquire(owner, "server", "", lowered)
	if !ok {
		t.Fatal("completed worker retained slot")
	}
	next()
}
func TestAdmissionConcurrentReleaseAndBounds(t *testing.T) {
	for _, cfg := range []admission.Settings{{Total: 2, PerServer: 1, PerPrincipal: 2}, {Total: 1, PerServer: 1, PerPrincipal: 1}} {
		h := &Handler{}
		r, ok := h.acquire(Owner{Principal: "a"}, "s", "", cfg)
		if !ok {
			t.Fatal("first")
		}
		if _, ok = h.acquire(Owner{Principal: "b"}, "s", "", cfg); ok {
			t.Fatal("server/total limit ignored")
		}
		slot := newAdmissionSlot(r)
		a, b := slot.releaseOnce(), slot.retain()
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Go(func() { a(); b() })
		}
		wg.Wait()
		if len(h.budgets) != 0 {
			t.Fatal("concurrent release leaked")
		}
	}
}
