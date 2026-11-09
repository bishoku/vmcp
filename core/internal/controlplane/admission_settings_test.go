package controlplane_test

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/store"
	"testing"
)

func TestAdmissionSettingsOperatorAndRevision(t *testing.T) {
	a := setup(t)
	secret, id := payloadReader(t, a)
	headers := map[string]string{"Authorization": "Bearer " + secret}
	code, _, _ := a.do("GET", "/api/v1/admission-settings", nil, headers)
	if code != 403 {
		t.Fatal("unassigned reader", code)
	}
	role := assignmentCall(t, a, 200, "GET", "/api/v1/roles/platform-operator", nil, "")
	grant := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", assignmentInput(role["id"].(string), id, map[string]any{"kind": "tenant"}), "")
	code, _, settings := a.do("GET", "/api/v1/admission-settings", nil, headers)
	if code != 200 || settings["perPrincipal"] != float64(16) {
		t.Fatal("defaults", code, settings)
	}
	before, err := store.LoadSnapshot(context.Background(), a.pool, a.control.Store.Box)
	if err != nil {
		t.Fatal(err)
	}
	settings["perPrincipal"] = float64(20)
	code, _, saved := a.do("PUT", "/api/v1/admission-settings", settings, headers)
	if code != 200 || saved["perPrincipal"] != float64(20) {
		t.Fatal("operator update", code, saved)
	}
	after, err := store.LoadSnapshot(context.Background(), a.pool, a.control.Store.Box)
	if err != nil || after.Admission.PerPrincipal != 20 || after.Revision <= before.Revision {
		t.Fatal("snapshot propagation", after, err)
	}
	code, _, _ = a.do("PUT", "/api/v1/admission-settings", settings, headers)
	if code != 409 {
		t.Fatal("lost update", code)
	}
	for _, value := range []any{0, -1, 1025, 1.5} {
		saved["perPrincipal"] = value
		code, _, _ = a.do("PUT", "/api/v1/admission-settings", saved, headers)
		if code != 422 && code != 400 {
			t.Fatal("invalid value accepted", value, code)
		}
	}
	assignmentCall(t, a, 204, "DELETE", "/api/v1/role-assignments/"+grant["id"].(string), nil, "\"1\"")
	saved["perPrincipal"] = float64(20)
	code, _, _ = a.do("PUT", "/api/v1/admission-settings", saved, headers)
	if code != 403 {
		t.Fatal("revoked operator saved", code)
	}
}
