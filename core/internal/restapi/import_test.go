package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const importFixture = `openapi: 3.0.3
info: {title: Orders}
servers: [{url: 'https://orders.example/api'}]
security: [{service: []}]
paths:
  /orders/{id}:
    parameters:
      - {$ref: '#/components/parameters/id'}
    get:
      operationId: getOrder
      summary: Retrieve order
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Order'}
    post:
      operationId: updateOrder
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Order'}
      responses:
        '204': {description: Updated}
  /upload:
    post:
      requestBody:
        content:
          multipart/form-data:
            schema: {type: object}
      responses: {'204': {description: OK}}
components:
  parameters:
    id:
      name: id
      in: path
      required: true
      schema: {type: integer, minimum: 1}
  schemas:
    Order:
      type: object
      required: [id, name]
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string, nullable: true}
        total: {type: number, minimum: 0, exclusiveMinimum: true}
        token: {type: string, default: DO_NOT_COPY, example: DO_NOT_COPY}
`

func TestImportOpenAPILocalReferencesAndSchemaConversion(t *testing.T) {
	result, err := Import(importFixture)
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Orders" || len(result.Operations) != 3 || result.Servers[0] != "https://orders.example/api" {
		t.Fatalf("%+v", result)
	}
	get, post, upload := result.Operations[0], result.Operations[1], result.Operations[2]
	if len(get.Errors) > 0 || get.Operation == nil || get.Operation.ReadOnly || len(get.Warnings) == 0 || len(post.Errors) > 0 || upload.Operation != nil || len(upload.Errors) == 0 {
		t.Fatalf("%+v", result.Operations)
	}
	if get.Operation.Parameters[0].Name != "id" || !get.Operation.Parameters[0].Required {
		t.Fatal("path parameter missing")
	}
	compiled, err := Compile("https://example.com", Config{Auth: Auth{Type: "none"}, TimeoutSeconds: 30, Operations: []Operation{*post.Operation}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiled.Build("https://example.com", "updateOrder", json.RawMessage(`{"id":1,"body":{"name":null,"total":2}}`), nil, Identity{}); err != nil {
		t.Fatal("read-only request property was required", err)
	}
	if _, err := compiled.Build("https://example.com", "updateOrder", json.RawMessage(`{"id":1,"body":{"name":"a","total":0}}`), nil, Identity{}); err == nil {
		t.Fatal("exclusiveMinimum lost")
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "DO_NOT_COPY") || strings.Contains(string(encoded), `"$ref"`) {
		t.Fatal("examples or references leaked into imported operations")
	}
}
func TestImportSwaggerAndInheritedOverrides(t *testing.T) {
	document := `{"swagger":"2.0","info":{"title":"Legacy"},"host":"legacy.example","basePath":"/v2","schemes":["https"],"consumes":["application/json"],"produces":["application/json"],"paths":{"/items":{"parameters":[{"name":"q","in":"query","type":"integer"}],"post":{"operationId":"create item","parameters":[{"name":"q","in":"query","type":"string"},{"name":"payload","in":"body","required":false,"schema":{"$ref":"#/definitions/Input"}}],"responses":{"201":{"description":"created","schema":{"type":"object"}},"204":{"description":"empty"}}}}},"definitions":{"Input":{"type":"object","properties":{"name":{"type":"string"}}}}}`
	result, err := Import(document)
	if err != nil {
		t.Fatal(err)
	}
	candidate := result.Operations[0]
	if candidate.Operation == nil {
		t.Fatal(candidate.Errors)
	}
	op := candidate.Operation
	if op.Name != "create_item" || len(op.Parameters) != 1 || !strings.Contains(string(op.Parameters[0].Schema), "string") || op.BodySchema == nil || len(op.SuccessStatuses) != 2 || len(candidate.Warnings) == 0 {
		t.Fatalf("%+v", candidate)
	}
	if result.Servers[0] != "https://legacy.example/v2" {
		t.Fatal(result.Servers)
	}
}
func TestImportUntrustedDocumentsAndUnsupportedOperations(t *testing.T) {
	for _, document := range []string{
		`openapi: 9.0.0`,
		`{"openapi":"3.1.0","jsonSchemaDialect":"https://example.com/custom","paths":{}}`,
		`{"openapi":"3.1.0","paths": !custom {"/x":{"get":{"responses":{"204":{}}}}}}`,
		"openapi: 3.1.0\npaths: {}\npaths: {}",
		"openapi: 3.1.0\npaths: &p {}\ncopy: *p",
		"openapi: 3.1.0\npaths: {}\n---\npaths: {}",
		strings.Repeat("x", MaxImportBytes+1),
	} {
		if _, err := Import(document); err == nil {
			t.Fatal("unsafe or invalid document accepted")
		}
	}
	for _, schema := range []string{`{"$ref":"http://127.0.0.1/secret"}`, `{"$ref":"file:///etc/passwd"}`, `{"$ref":"#/components/schemas/Loop"}`, `{"type":"string","writeOnly":true}`, `{"$dynamicRef":"#anchor"}`} {
		document := `{"openapi":"3.1.0","paths":{"/x":{"get":{"responses":{"200":{"content":{"application/json":{"schema":` + schema + `}}}}}}},"components":{"schemas":{"Loop":{"type":"object","properties":{"next":{"$ref":"#/components/schemas/Loop"}}}}}}`
		result, err := Import(document)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Operations) != 1 || result.Operations[0].Operation != nil || len(result.Operations[0].Errors) == 0 {
			t.Fatal("unsupported schema published", schema)
		}
	}
}
func TestImportNameCollisionsAreStableAndUnique(t *testing.T) {
	result, err := Import(`{"openapi":"3.1.0","paths":{"/a":{"get":{"operationId":"lookup_2","responses":{"204":{}}}},"/b":{"get":{"operationId":"lookup","responses":{"204":{}}}},"/c":{"get":{"operationId":"lookup","responses":{"204":{}}}},"/d":{"get":{"operationId":"lookup","responses":{"204":{}}}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, candidate := range result.Operations {
		if candidate.Operation == nil || seen[candidate.Operation.Name] {
			t.Fatalf("name collision: %+v", result)
		}
		seen[candidate.Operation.Name] = true
	}
	if !seen["lookup_3"] || !seen["lookup_4"] {
		t.Fatal(seen)
	}
}
func TestImportComplexityAndExternalPathReferences(t *testing.T) {
	result, err := Import(`{"openapi":"3.1.0","paths":{"/remote":{"$ref":"https://example.com/paths","get":{"responses":{"204":{}}}},"/ok":{"get":{"responses":{"204":{}}}}}}`)
	if err != nil || len(result.Operations) != 2 || result.Operations[0].Operation == nil || len(result.Operations[1].Errors) == 0 {
		t.Fatalf("%+v %v", result, err)
	}
	paths := map[string]any{}
	for n := 0; n < 1001; n++ {
		paths[fmt.Sprintf("/p%d", n)] = map[string]any{"get": map[string]any{"responses": map[string]any{"204": map[string]any{}}}}
	}
	document := raw(map[string]any{"openapi": "3.1.0", "paths": paths})
	if _, err := Import(string(document)); err == nil {
		t.Fatal("operation limit not enforced")
	}
}
func TestImportNullableRetainsEnumAndLargeNumbers(t *testing.T) {
	document := `{"openapi":"3.0.3","paths":{"/x":{"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"string","nullable":true,"enum":["allowed"]}}}},"responses":{"200":{"content":{"application/json":{"schema":{"type":"integer","minimum":9007199254740993}}}}}}}}}`
	result, err := Import(document)
	if err != nil {
		t.Fatal(err)
	}
	op := result.Operations[0].Operation
	if op == nil {
		t.Fatal(result.Operations[0].Errors)
	}
	compiled := compile(t, "https://example.com", Config{Auth: Auth{Type: "none"}, TimeoutSeconds: 30, Operations: []Operation{*op}})
	if _, err := compiled.Build("https://example.com", op.Name, json.RawMessage(`{"body":null}`), nil, Identity{}); err == nil {
		t.Fatal("nullable bypassed enum restriction")
	}
	if !strings.Contains(string(op.ResponseSchema), "9007199254740993") {
		t.Fatal("integer rounded")
	}
}

func TestImportNeverFetchesServersOrExternalReferences(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); _, _ = w.Write([]byte(`{"type":"object"}`)) }))
	defer server.Close()
	doc := fmt.Sprintf(`{"openapi":"3.1.0","servers":[{"url":%q}],"paths":{"/x":{"get":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":%q}}}}}}}}}`, server.URL, server.URL+"/schema")
	result, err := Import(doc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Operations[0].Operation != nil || len(result.Operations[0].Errors) == 0 || hits.Load() != 0 {
		t.Fatal("external reference fetched or accepted")
	}
}

func TestImportBlockedResponseShowsPropertyLocation(t *testing.T) {
	result, err := Import(`{"openapi":"3.1.0","paths":{"/orders":{"get":{"operationId":"orders","responses":{"200":{"content":{"application/json":{"schema":{"type":"object","properties":{"customer":{"type":"object","properties":{"secret":{"type":"string","writeOnly":true}}}}}}}}}}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || len(result.Operations[0].Errors) != 1 || result.Operations[0].Operation != nil {
		t.Fatal("unsupported operation imported")
	}
	detail := result.Operations[0].Errors[0]
	if !strings.Contains(detail, "response HTTP 200") || !strings.Contains(detail, "properties.customer") || !strings.Contains(detail, "properties.secret") {
		t.Fatal(detail)
	}
}
