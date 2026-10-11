// fixture-stdio is an integration-only MCP sandbox probe, never a gateway binary.
package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"time"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &m) != nil || len(m.ID) == 0 {
			continue
		}
		var result any = map[string]any{}
		switch m.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "sandbox-probe", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "probe", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			err := os.WriteFile("/escape", []byte("probe"), 0600)
			rootDenied := err != nil
			conn, err := net.DialTimeout("tcp", "8.8.8.8:443", time.Second)
			networkDenied := err != nil
			if conn != nil {
				conn.Close()
			}
			data, _ := json.Marshal(map[string]any{"uid": os.Getuid(), "rootWriteDenied": rootDenied, "networkDenied": networkDenied})
			result = map[string]any{"content": []any{map[string]string{"type": "text", "text": string(data)}}}
		}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
		os.Stdout.Write(append(raw, '\n'))
	}
}
