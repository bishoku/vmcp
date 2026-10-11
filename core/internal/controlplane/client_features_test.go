package controlplane_test

import "testing"

func TestClientFeaturePermissionETagsAndGitOps(t *testing.T) {
	a := setup(t)
	status, headers, body := a.do("POST", "/api/v1/virtual-servers", map[string]any{"slug": "client-features", "name": "Client Features", "clientFeatures": map[string]bool{"sampling": true, "roots": true}}, nil)
	if status != 201 {
		t.Fatalf("permissions %d %+v", status, body)
	}
	features := body["clientFeatures"].(map[string]any)
	if features["sampling"] != true || features["elicitation"] != false {
		t.Fatal("client permission defaults")
	}
	if status, _, _ := a.do("PATCH", "/api/v1/virtual-servers/client-features", map[string]any{"clientFeatures": map[string]bool{"elicitation": true}}, map[string]string{"If-Match": "\"9999\""}); status != 412 {
		t.Fatal("stale feature-policy update accepted")
	}
	status, _, body = a.do("PATCH", "/api/v1/virtual-servers/client-features", map[string]any{"name": "Retained permissions"}, map[string]string{"If-Match": headers.Get("ETag")})
	if status != 200 || body["clientFeatures"].(map[string]any)["sampling"] != true {
		t.Fatal("unrelated patch cleared permissions")
	}
	_, _, document := a.do("GET", "/api/v1/export", nil, nil)
	if status, _, result := a.do("POST", "/api/v1/apply?refreshTools=false", document, nil); status != 200 {
		t.Fatalf("feature GitOps %d %+v", status, result)
	}
	_, _, body = a.do("GET", "/api/v1/virtual-servers/client-features", nil, nil)
	if body["clientFeatures"].(map[string]any)["roots"] != true {
		t.Fatal("export/apply dropped roots permission")
	}
}
