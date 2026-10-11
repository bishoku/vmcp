package stdioworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWorkerBridgeLifecycleAndIsolation(t *testing.T) {
	profile := Profile{Image: "fixture@sha256:" + strings.Repeat("a", 64)}
	worker, err := New(map[string]Profile{"fixture": profile, "other": profile})
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	worker.Launch = func(ctx context.Context, p Profile, id string) (*Process, error) {
		inputR, inputW := io.Pipe()
		outputR, outputW := io.Pipe()
		var once sync.Once
		stop := func() { once.Do(func() { inputR.Close(); inputW.Close(); outputR.Close(); outputW.Close() }) }
		go func() {
			scanner := bufio.NewScanner(inputR)
			for scanner.Scan() {
				var m map[string]any
				if json.Unmarshal(scanner.Bytes(), &m) != nil {
					return
				}
				if m["id"] == nil {
					continue
				}
				result := map[string]any{"echo": "ok"}
				if m["method"] == "initialize" {
					result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "serverInfo": map[string]string{"name": "fixture", "version": "1"}}
				}
				raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m["id"], "result": result})
				outputW.Write(append(raw, '\n'))
			}
		}()
		return &Process{Input: inputW, Output: outputR, Stop: stop}, nil
	}
	server := httptest.NewServer(worker)
	defer server.Close()
	send := func(path, id, method, body string) *http.Response {
		t.Helper()
		request, _ := http.NewRequest(method, server.URL+path, bytes.NewBufferString(body))
		request.Header.Set("Mcp-Session-Id", id)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := send("/mcp/fixture", "", "POST", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	id := response.Header.Get("Mcp-Session-Id")
	response.Body.Close()
	if response.StatusCode != 200 || len(id) != 48 {
		t.Fatal("initialization failed")
	}
	response = send("/mcp/other", id, "POST", `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal("cross-profile session accepted")
	}
	response = send("/mcp/fixture", id, "POST", `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !bytes.Contains(raw, []byte(`"echo":"ok"`)) {
		t.Fatal("stdio response not relayed")
	}
	worker.Sweep(time.Now().Add(16 * time.Minute))
	response = send("/mcp/fixture", id, "DELETE", "")
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal("expired session survived")
	}
}
func TestWorkerRejectsNonAllowlistedCommands(t *testing.T) {
	for _, image := range []string{"alpine:latest", "$(touch /tmp/escape)", "-v /:/host", strings.Repeat("a", 64)} {
		if _, err := New(map[string]Profile{"fixture": {Image: image}}); err == nil {
			t.Fatal("mutable/command image accepted")
		}
	}
	worker, _ := New(map[string]Profile{"fixture": {Image: "fixture@sha256:" + strings.Repeat("a", 64)}})
	defer worker.Close()
	response := httptest.NewRecorder()
	worker.ServeHTTP(response, httptest.NewRequest("POST", "/mcp/not-allowed", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)))
	if response.Code != 404 {
		t.Fatal("unknown command accepted")
	}
}

func TestRealDockerSandbox(t *testing.T) {
	image := os.Getenv("VMCP_TEST_STDIO_IMAGE")
	if image == "" {
		return
	}
	worker, err := New(map[string]Profile{"probe": {Image: image}})
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	server := httptest.NewServer(worker)
	defer server.Close()
	post := func(id, raw string) *http.Response {
		t.Helper()
		request, _ := http.NewRequest("POST", server.URL+"/mcp/probe", strings.NewReader(raw))
		request.Header.Set("Mcp-Session-Id", id)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := post("", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	id := response.Header.Get("Mcp-Session-Id")
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("container initialization failed: %d %s", response.StatusCode, body)
	}
	response = post(id, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"probe"}}`)
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	for _, expected := range []string{`\"uid\":65532`, `\"rootWriteDenied\":true`, `\"networkDenied\":true`} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("sandbox check failed: %s", body)
		}
	}
	output, err := exec.Command("docker", "inspect", "vmcp-stdio-"+id, "--format", `{{.HostConfig.ReadonlyRootfs}} {{.HostConfig.NetworkMode}} {{.HostConfig.Memory}} {{.HostConfig.PidsLimit}} {{.HostConfig.NanoCpus}}`).CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "true none 134217728 64 500000000" {
		t.Fatalf("container limits missing: %s %v", output, err)
	}
}

func TestSecretInjectionIsOperatorScoped(t *testing.T) {
	root := t.TempDir()
	t.Setenv("VMCP_STDIO_SECRET_ROOT", root)
	file := filepath.Join(root, "profile-secret")
	if err := os.WriteFile(file, []byte("TOKEN=private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("a", 64)
	worker, err := New(map[string]Profile{"p": {Image: image, SecretFile: "profile-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	if worker.Profiles["p"].envFile != file {
		t.Fatal("secret mapping lost")
	}
	if _, err := New(map[string]Profile{"p": {Image: image, SecretFile: "../escape"}}); err == nil {
		t.Fatal("secret path escape accepted")
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(map[string]Profile{"p": {Image: image, SecretFile: "profile-secret"}}); err == nil {
		t.Fatal("public secret file accepted")
	}
}

func TestWorkerRestartDoesNotReexecuteOwnedSessions(t *testing.T) {
	profile := Profile{Image: "sha256:" + strings.Repeat("a", 64)}
	worker, _ := New(map[string]Profile{"p": profile})
	defer worker.Close()
	launched := false
	worker.Launch = func(context.Context, Profile, string) (*Process, error) {
		launched = true
		return nil, fmt.Errorf("must not launch")
	}
	request := httptest.NewRequest("POST", "/mcp/p", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call"}`))
	request.Header.Set("Mcp-Session-Id", "another-worker-or-pre-restart-session")
	response := httptest.NewRecorder()
	worker.ServeHTTP(response, request)
	if response.Code != 404 || launched {
		t.Fatal("lost worker session was reexecuted")
	}
}

func TestWorkerShutdownStopsProcessesConcurrently(t *testing.T) {
	worker, _ := New(map[string]Profile{"p": {Image: "sha256:" + strings.Repeat("a", 64)}})
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	for _, id := range []string{"a", "b", "c"} {
		worker.sessions[id] = &session{done: make(chan struct{}), process: &Process{Stop: func() { entered <- struct{}{}; <-release }}}
	}
	done := make(chan struct{})
	go func() { worker.Close(); close(done) }()
	for i := 0; i < 3; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("shutdown serialized process termination")
		}
	}
	once.Do(func() { close(release) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown failed to finish")
	}
}
