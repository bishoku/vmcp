// Package localtools serves authored text through MCP without an upstream connection.
package localtools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"text/template"
	"text/template/parse"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

type Parameter struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Required bool            `json:"required,omitempty"`
	Choices  []string        `json:"choices,omitempty"`
	Default  json.RawMessage `json:"default,omitempty"`
}
type Definition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  []Parameter `json:"parameters"`
	Response    string      `json:"response"`
}
type Config struct {
	Tools []Definition `json:"tools"`
}

var toolNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var nameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

type Compiled struct {
	Tools   []mcp.Tool
	entries map[string]entry
}
type entry struct {
	def  Definition
	tmpl *template.Template
}

func Compile(c Config) (*Compiled, error) {
	out := &Compiled{entries: map[string]entry{}}
	if c.Tools == nil {
		return nil, errors.New("provide a tools array")
	}
	if len(c.Tools) > 128 {
		return nil, errors.New("up to 128 tools allowed")
	}
	for _, d := range c.Tools {
		if !toolNameRE.MatchString(d.Name) || out.entries[d.Name].tmpl != nil {
			return nil, fmt.Errorf("invalid or duplicate tool name %q", d.Name)
		}
		if len(d.Response) > 65536 || len(d.Description) > 4096 || len(d.Parameters) > 32 {
			return nil, errors.New("tool definition exceeds size limit")
		}
		props := map[string]any{}
		required := []string{}
		for _, p := range d.Parameters {
			if !nameRE.MatchString(p.Name) || props[p.Name] != nil {
				return nil, fmt.Errorf("invalid or duplicate parameter %q", p.Name)
			}
			s := map[string]any{"type": p.Type}
			switch p.Type {
			case "string", "boolean":
			case "string[]":
				s["type"] = "array"
				s["maxItems"] = 128
				s["items"] = map[string]string{"type": "string"}
			default:
				return nil, fmt.Errorf("%s: unsupported type", p.Name)
			}
			if len(p.Choices) > 0 {
				if p.Type != "string" {
					return nil, fmt.Errorf("%s: choices require string type", p.Name)
				}
				s["enum"] = p.Choices
			}
			if p.Required && p.Default != nil {
				return nil, fmt.Errorf("%s: defaults apply to optional parameters", p.Name)
			}
			if p.Default != nil {
				v, err := p.decode(p.Default)
				if err != nil {
					return nil, err
				}
				s["default"] = v
			}
			if p.Required {
				required = append(required, p.Name)
			}
			props[p.Name] = s
		}
		tmpl, err := template.New(d.Name).Parse(d.Response)
		if err != nil {
			return nil, err
		}
		if len(tmpl.Templates()) != 1 {
			return nil, errors.New("named templates are unsupported")
		}
		if err = checkTree(tmpl.Tree.Root, false, true); err != nil {
			return nil, err
		}
		schema, _ := json.Marshal(map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false})
		out.Tools = append(out.Tools, mcp.Tool{Name: d.Name, Description: d.Description, InputSchema: schema})
		out.entries[d.Name] = entry{d, tmpl}
	}
	return out, nil
}

// Restrict loops to direct array arguments, and expressions to side-effect-free comparisons.
func checkTree(n parse.Node, inLoop, root bool) error {
	switch x := n.(type) {
	case *parse.ListNode:
		if x != nil {
			for _, v := range x.Nodes {
				if err := checkTree(v, inLoop, root); err != nil {
					return err
				}
			}
		}
	case *parse.IfNode:
		for _, v := range []parse.Node{x.Pipe, x.List, x.ElseList} {
			if err := checkTree(v, inLoop, root); err != nil {
				return err
			}
		}
	case *parse.RangeNode:
		if inLoop || len(x.Pipe.Cmds) != 1 || len(x.Pipe.Cmds[0].Args) != 1 {
			return errors.New("range requires a single array parameter")
		}
		f, ok := x.Pipe.Cmds[0].Args[0].(*parse.FieldNode)
		if !ok || len(f.Ident) != 2 || f.Ident[0] != "Args" {
			return errors.New("use range .Args.parameter")
		}
		for _, v := range []parse.Node{x.Pipe, x.List, x.ElseList} {
			if err := checkTree(v, true, false); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return checkTree(x.Pipe, inLoop, root)
	case *parse.PipeNode:
		if len(x.Decl) > 0 {
			return errors.New("variable assignment is unsupported")
		}
		for _, c := range x.Cmds {
			if err := checkTree(c, inLoop, root); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		for _, a := range x.Args {
			if err := checkTree(a, inLoop, root); err != nil {
				return err
			}
		}
	case *parse.IdentifierNode:
		switch x.Ident {
		case "eq", "ne", "and", "or", "not", "len":
		default:
			return fmt.Errorf("unsupported function %s", x.Ident)
		}
	case *parse.VariableNode:
		if len(x.Ident) != 3 || x.Ident[0] != "$" || x.Ident[1] != "Args" {
			return errors.New("select a root argument using $.Args.name")
		}
	case *parse.DotNode:
		if root {
			return errors.New("select an argument using .Args.name")
		}
	case *parse.TextNode, *parse.FieldNode, *parse.StringNode, *parse.BoolNode, *parse.NumberNode:
	default:
		return fmt.Errorf("unsupported template expression %s", n.String())
	}
	return nil
}
func (p Parameter) decode(raw json.RawMessage) (any, error) {
	var v any
	if json.Unmarshal(raw, &v) == nil {
		switch x := v.(type) {
		case string:
			if p.Type == "string" {
				if len(p.Choices) == 0 {
					return x, nil
				}
				for _, s := range p.Choices {
					if s == x {
						return x, nil
					}
				}
			}
		case bool:
			if p.Type == "boolean" {
				return x, nil
			}
		case []any:
			if p.Type == "string[]" && len(x) <= 128 {
				for _, item := range x {
					if _, ok := item.(string); !ok {
						return nil, fmt.Errorf("%s: expected strings", p.Name)
					}
				}
				return x, nil
			}
		}
	}
	return nil, fmt.Errorf("%s: invalid %s value", p.Name, p.Type)
}

type output struct{ bytes.Buffer }

func (o *output) Write(p []byte) (int, error) {
	if o.Len()+len(p) > 256<<10 {
		return 0, errors.New("response exceeds 256 KiB")
	}
	return o.Buffer.Write(p)
}
func (c *Compiled) Render(name string, raw json.RawMessage) (string, error) {
	e, ok := c.entries[name]
	if !ok {
		return "", errors.New("unknown local tool")
	}
	args := map[string]json.RawMessage{}
	if len(raw) > 65536 {
		return "", errors.New("arguments exceed 64 KiB")
	}
	if len(raw) > 0 && (json.Unmarshal(raw, &args) != nil || args == nil) {
		return "", errors.New("expected an arguments object")
	}
	values := map[string]any{}
	for _, p := range e.def.Parameters {
		v, exists := args[p.Name]
		delete(args, p.Name)
		if !exists {
			if p.Required {
				return "", fmt.Errorf("%s is required", p.Name)
			}
			v = p.Default
			if v == nil {
				continue
			}
		}
		value, err := p.decode(v)
		if err != nil {
			return "", err
		}
		values[p.Name] = value
	}
	for k := range args {
		return "", fmt.Errorf("unknown parameter %s", k)
	}
	var out output
	if err := e.tmpl.Execute(&out, struct{ Args map[string]any }{values}); err != nil {
		return "", err
	}
	return out.String(), nil
}
