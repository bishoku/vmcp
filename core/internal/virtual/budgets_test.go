package virtual

import "testing"

func TestAdmissionBudgetReclaimsIdentity(t *testing.T) {
	h := &Handler{}
	var releases []func()
	for i := 0; i < 16; i++ {
		release, ok := h.acquire(Owner{}, "server")
		if !ok {
			t.Fatal("early rejection")
		}
		releases = append(releases, release)
	}
	if _, ok := h.acquire(Owner{}, "server"); ok {
		t.Fatal("anonymous concurrency unbounded")
	}
	for _, release := range releases {
		release()
	}
	if len(h.budgets) != 0 {
		t.Fatal("identity counters leaked")
	}
}

func TestSessionCapacityAndClose(t *testing.T) {
	m := NewManager()
	for i := 0; i < 10000; i++ {
		if _, err := m.Create("s", "v"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Create("s", "v"); err == nil {
		t.Fatal("session capacity unbounded")
	}
	m.Close()
	if m.Len() != 0 {
		t.Fatal("close retained local sessions")
	}
	if _, err := m.Create("s", "v"); err == nil {
		t.Fatal("admission after close")
	}
}
