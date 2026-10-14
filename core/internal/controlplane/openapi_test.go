package controlplane_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func TestOpenAPIDownloads(t *testing.T) {
	a := setup(t)
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			path := "/api/v1/openapi." + format
			res, err := http.Get(a.srv.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("unauthenticated: %d", res.StatusCode)
			}
			req, _ := http.NewRequest(http.MethodGet, a.srv.URL+path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			res, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("download: %d", res.StatusCode)
			}
			if got := res.Header.Get("Content-Disposition"); got != `attachment; filename="vmcp-openapi.`+format+`"` {
				t.Fatalf("disposition: %s", got)
			}
			var body bytes.Buffer
			if _, err := body.ReadFrom(res.Body); err != nil {
				t.Fatal(err)
			}
			if format == "json" {
				if res.Header.Get("Content-Type") != "application/json" {
					t.Fatal("incorrect JSON content type")
				}
				var spec map[string]any
				if err := json.Unmarshal(body.Bytes(), &spec); err != nil {
					t.Fatal(err)
				}
				if spec["openapi"] != "3.1.0" {
					t.Fatal("incorrect specification")
				}
				paths := spec["paths"].(map[string]any)
				if paths["/api/v1/upstreams"] == nil || paths[path] == nil {
					t.Fatal("missing API paths")
				}
			} else {
				if res.Header.Get("Content-Type") != "application/yaml" {
					t.Fatal("incorrect YAML content type")
				}
				source, err := os.ReadFile("../../../api/openapi.yaml")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(body.Bytes(), source) {
					t.Fatal("YAML differs from canonical source")
				}
			}
		})
	}
}
