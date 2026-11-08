// Package toolschema validates MCP arguments without network schema resolution.
package toolschema

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var ErrSchema = errors.New("tool input schema is invalid, unsupported or exceeds limits")
var ErrArguments = errors.New("arguments do not match the tool input schema")

type Compiled struct {
	schema *jsonschema.Schema
	err    error
}

var cache = struct {
	sync.Mutex
	entries map[[32]byte]*Compiled
	order   [][32]byte
}{entries: map[[32]byte]*Compiled{}}

// Compile caches at most 256 small schemas, including failures. Snapshot owners
// retain their immutable result. Only bounded, acyclic local JSON-pointer references are supported.
func Compile(raw []byte) *Compiled {
	if len(raw) > 65536 {
		return &Compiled{err: ErrSchema}
	}
	key := sha256.Sum256(raw)
	cache.Lock()
	defer cache.Unlock()
	if c := cache.entries[key]; c != nil {
		return c
	}
	c := compile(raw)
	if len(cache.order) == 256 {
		delete(cache.entries, cache.order[0])
		cache.order = cache.order[1:]
	}
	cache.order = append(cache.order, key)
	cache.entries[key] = c
	return c
}
func compile(raw []byte) *Compiled {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return &Compiled{err: ErrSchema}
	}
	object, ok := value.(map[string]any)
	if !ok || object["type"] != "object" {
		return &Compiled{err: ErrSchema}
	}
	budget := 4096
	resolving := map[string]bool{}
	var bounded func(any, int) bool
	bounded = func(v any, depth int) bool {
		budget--
		if budget < 0 || depth > 32 {
			return false
		}
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				if k == "$dynamicRef" || k == "$recursiveRef" {
					return false
				}
				if k == "$ref" {
					ref, ok := v.(string)
					if !ok || !strings.HasPrefix(ref, "#/") || resolving[ref] {
						return false
					}
					pointer, err := url.PathUnescape(ref[2:])
					if err != nil {
						return false
					}
					target := value
					for _, part := range strings.Split(pointer, "/") {
						part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
						switch node := target.(type) {
						case map[string]any:
							var found bool
							target, found = node[part]
							if !found {
								return false
							}
						case []any:
							index, err := strconv.Atoi(part)
							if err != nil || index < 0 || index >= len(node) {
								return false
							}
							target = node[index]
						default:
							return false
						}
					}
					resolving[ref] = true
					valid := bounded(target, depth+1)
					delete(resolving, ref)
					if !valid {
						return false
					}
				}
				if !bounded(v, depth+1) {
					return false
				}
			}
		case []any:
			for _, v := range x {
				if !bounded(v, depth+1) {
					return false
				}
			}
		}
		return true
	}
	if !bounded(value, 0) {
		return &Compiled{err: ErrSchema}
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(nil)
	if compiler.AddResource("urn:vmcp:tool", value) != nil {
		return &Compiled{err: ErrSchema}
	}
	schema, err := compiler.Compile("urn:vmcp:tool")
	if err != nil {
		return &Compiled{err: ErrSchema}
	}
	return &Compiled{schema: schema}
}
func (c *Compiled) Validate(raw []byte) error {
	if c == nil || c.err != nil {
		return ErrSchema
	}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if len(raw) > 4<<20 {
		return ErrArguments
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || !json.Valid(raw) {
		return ErrArguments
	}
	budget := 100000
	var bounded func(any, int) bool
	bounded = func(v any, depth int) bool {
		budget--
		if budget < 0 || depth > 64 {
			return false
		}
		switch x := v.(type) {
		case map[string]any:
			for _, v := range x {
				if !bounded(v, depth+1) {
					return false
				}
			}
		case []any:
			for _, v := range x {
				if !bounded(v, depth+1) {
					return false
				}
			}
		}
		return true
	}
	if !bounded(value, 0) || c.schema.Validate(value) != nil {
		return ErrArguments
	}
	return nil
}
