package controlplane

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
)

func TestEmbeddedOpenAPISpecifications(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			// Exercise routing and bearer authentication without a database dependency.
			s := &Server{AdminToken: "test"}
			req := httptest.NewRequest("GET", "/api/v1/openapi."+format, nil)
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("status: %d", rec.Code)
			}
			if rec.Header().Get("Content-Disposition") != `attachment; filename="vmcp-openapi.`+format+`"` {
				t.Fatal("missing download filename")
			}
			if format == "json" {
				var spec map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
					t.Fatal(err)
				}
				if spec["openapi"] != "3.1.0" {
					t.Fatal("incorrect spec version")
				}
			} else {
				source, err := os.ReadFile("../../../api/openapi.yaml")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(source, rec.Body.Bytes()) {
					t.Fatal("embedded YAML differs from source")
				}
			}
		})
	}
}
