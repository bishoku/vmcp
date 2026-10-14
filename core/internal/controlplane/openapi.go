package controlplane

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed spec/openapi.json spec/openapi.yaml
var openAPISpecs embed.FS

func (s *Server) getOpenAPI(w http.ResponseWriter, r *http.Request) error {
	format := "yaml"
	contentType := "application/yaml"
	if strings.HasSuffix(r.URL.Path, ".json") {
		format = "json"
		contentType = "application/json"
	}
	data, err := openAPISpecs.ReadFile("spec/openapi." + format)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="vmcp-openapi.`+format+`"`)
	_, err = w.Write(data)
	return err
}
