// Package stdioworker bridges MCP to operator-allowlisted isolated containers.
// It is linked into the separate worker binary, never the API process.
package stdioworker

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Profile struct {
	Image      string `json:"image"`
	SecretFile string `json:"secretFile,omitempty"`
	envFile    string
}
type Process struct {
	Input  io.WriteCloser
	Output io.ReadCloser
	Stop   func()
}
type Launcher func(context.Context, Profile, string) (*Process, error)
type Worker struct {
	Profiles map[string]Profile
	Launch   Launcher
	mu       sync.Mutex
	sessions map[string]*session
	closed   bool
	capacity chan struct{}
}
type session struct {
	profile string
	id      string
	process *Process
	write   sync.Mutex
	mu      sync.Mutex
	call    sync.Mutex
	get     sync.Mutex
	active  chan []byte
	events  chan []byte
	done    chan struct{}
	once    sync.Once
	touched time.Time
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var imagePattern = regexp.MustCompile(`^(?:[a-zA-Z0-9./:_-]+@)?sha256:[a-f0-9]{64}$`)

func New(profiles map[string]Profile) (*Worker, error) {
	if len(profiles) == 0 || len(profiles) > 32 {
		return nil, errors.New("worker requires 1..32 allowlisted profiles")
	}
	copy := map[string]Profile{}
	for name, profile := range profiles {
		if !namePattern.MatchString(name) || !imagePattern.MatchString(profile.Image) {
			return nil, errors.New("worker profiles require names and immutable image digests")
		}
		if profile.SecretFile != "" {
			if !namePattern.MatchString(profile.SecretFile) {
				return nil, errors.New("invalid worker secret name")
			}
			root := os.Getenv("VMCP_STDIO_SECRET_ROOT")
			if root == "" {
				root = "/run/vmcp-secrets"
			}
			if !filepath.IsAbs(root) {
				return nil, errors.New("worker secret root must be absolute")
			}
			file := filepath.Join(root, profile.SecretFile)
			info, err := os.Lstat(file)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
				return nil, errors.New("worker secret file must be a private regular file")
			}
			profile.envFile = file
		}
		copy[name] = profile
	}
	return &Worker{Profiles: copy, Launch: Docker, sessions: map[string]*session{}, capacity: make(chan struct{}, 64)}, nil
}

// Docker uses no shell, host mounts, inherited credentials or arbitrary arguments.
// The worker host must provide a separately administered rootless Docker daemon.
func Docker(ctx context.Context, profile Profile, id string) (*Process, error) {
	if !imagePattern.MatchString(profile.Image) || !namePattern.MatchString(id) {
		return nil, errors.New("invalid container profile")
	}
	args := []string{"run", "--rm", "-i", "--name", "vmcp-stdio-" + id, "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=128m", "--memory-swap=128m", "--cpus=0.5", "--user=65532:65532", "--tmpfs=/tmp:rw,noexec,nosuid,size=16m"}
	if profile.envFile != "" {
		args = append(args, "--env-file", profile.envFile)
	}
	args = append(args, profile.Image)
	command := exec.CommandContext(ctx, "docker", args...)
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, errors.New("isolated process failed to start")
	}
	exited := make(chan struct{})
	go func() { _ = command.Wait(); close(exited) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			input.Close()
			bounded, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = exec.CommandContext(bounded, "docker", "rm", "-f", "vmcp-stdio-"+id).Run()
			select {
			case <-exited:
			case <-bounded.Done():
				if command.Process != nil {
					_ = command.Process.Kill()
				}
			}
			output.Close()
		})
	}
	return &Process{Input: input, Output: output, Stop: stop}, nil
}
func (s *session) stop() { s.once.Do(func() { close(s.done); s.process.Stop() }) }
func (w *Worker) Close() {
	w.mu.Lock()
	w.closed = true
	sessions := w.sessions
	w.sessions = map[string]*session{}
	w.mu.Unlock()
	var stopped sync.WaitGroup
	for _, s := range sessions {
		stopped.Add(1)
		go func(s *session) { defer stopped.Done(); s.stop() }(s)
	}
	stopped.Wait()
}
func (w *Worker) Sweep(now time.Time) {
	w.mu.Lock()
	expired := []*session{}
	for id, s := range w.sessions {
		s.mu.Lock()
		stale := now.Sub(s.touched) > 15*time.Minute
		s.mu.Unlock()
		select {
		case <-s.done:
			stale = true
		default:
		}
		if stale {
			delete(w.sessions, id)
			expired = append(expired, s)
		}
	}
	w.mu.Unlock()
	var stopped sync.WaitGroup
	for _, s := range expired {
		stopped.Add(1)
		go func(s *session) { defer stopped.Done(); s.stop() }(s)
	}
	stopped.Wait()
}
func (s *session) read() {
	scanner := bufio.NewScanner(s.process.Output)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	defer s.stop()
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		if !json.Valid(raw) {
			return
		}
		s.mu.Lock()
		target := s.active
		s.mu.Unlock()
		if target == nil {
			target = s.events
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case target <- raw:
			timer.Stop()
		case <-s.done:
			timer.Stop()
			return
		case <-timer.C:
			return
		}
	}
}
func (s *session) send(raw []byte) error {
	if !s.write.TryLock() {
		return errors.New("stdio write capacity")
	}
	done := make(chan error, 1)
	go func() {
		defer s.write.Unlock()
		_, err := s.process.Input.Write(append(append([]byte(nil), raw...), '\n'))
		done <- err
	}()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-s.done:
		return errors.New("worker session ended")
	case <-timer.C:
		s.stop()
		return errors.New("stdio write timeout")
	}
}

func (w *Worker) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	select {
	case w.capacity <- struct{}{}:
		defer func() { <-w.capacity }()
	default:
		http.Error(response, "worker HTTP capacity", 429)
		return
	}
	profileName := strings.TrimPrefix(request.URL.Path, "/mcp/")
	profile, ok := w.Profiles[profileName]
	if !ok || request.URL.RawQuery != "" {
		http.NotFound(response, request)
		return
	}
	if request.Method != http.MethodPost && request.Method != http.MethodGet && request.Method != http.MethodDelete {
		response.Header().Set("Allow", "POST, GET, DELETE")
		response.WriteHeader(405)
		return
	}
	id := request.Header.Get("Mcp-Session-Id")
	var raw []byte
	var message struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}
	if request.Method == http.MethodPost {
		var err error
		raw, err = io.ReadAll(http.MaxBytesReader(response, request.Body, 1<<20))
		if err != nil || json.Unmarshal(raw, &message) != nil || message.JSONRPC != "2.0" {
			http.Error(response, "invalid MCP message", 400)
			return
		}
	}
	w.mu.Lock()
	s := w.sessions[id]
	if w.closed {
		w.mu.Unlock()
		http.Error(response, "worker draining", 503)
		return
	}
	if s == nil && id == "" && request.Method == http.MethodPost && message.Method == "initialize" && len(message.ID) > 0 {
		if len(w.sessions) >= 32 {
			w.mu.Unlock()
			http.Error(response, "worker capacity", 429)
			return
		}
		token := make([]byte, 24)
		if _, err := rand.Read(token); err != nil {
			w.mu.Unlock()
			http.Error(response, "worker unavailable", 503)
			return
		}
		id = hex.EncodeToString(token)
		// Launch under the capacity lock, bounded independently of client lifetime.
		process, err := w.Launch(context.Background(), profile, id)
		if err != nil {
			w.mu.Unlock()
			http.Error(response, "worker unavailable", 503)
			return
		}
		s = &session{id: id, profile: profileName, process: process, events: make(chan []byte, 1), done: make(chan struct{}), touched: time.Now()}
		w.sessions[id] = s
		go s.read()
	}
	w.mu.Unlock()
	if s == nil || s.profile != profileName {
		http.NotFound(response, request)
		return
	}
	select {
	case <-s.done:
		http.NotFound(response, request)
		return
	default:
	}
	s.mu.Lock()
	s.touched = time.Now()
	s.mu.Unlock()
	if request.Method == http.MethodDelete {
		w.mu.Lock()
		delete(w.sessions, id)
		w.mu.Unlock()
		s.stop()
		response.WriteHeader(204)
		return
	}
	if request.Method == http.MethodGet {
		if !s.get.TryLock() {
			http.Error(response, "GET stream already active", 409)
			return
		}
		defer s.get.Unlock()
		w.stream(response, request, s, s.events, nil)
		return
	}
	if message.Method == "" || len(message.ID) == 0 {
		if err := s.send(raw); err != nil {
			http.Error(response, "worker unavailable", 503)
			return
		}
		response.WriteHeader(202)
		return
	}
	if !s.call.TryLock() {
		http.Error(response, "one active request per worker session", 429)
		return
	}
	defer s.call.Unlock()
	events := make(chan []byte, 1)
	s.mu.Lock()
	s.active = events
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active = nil; s.mu.Unlock() }()
	if err := s.send(raw); err != nil {
		http.Error(response, "worker unavailable", 503)
		return
	}
	response.Header().Set("Mcp-Session-Id", id)
	if message.Method == "initialize" {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			s.stop()
			http.Error(response, "initialization timeout", 504)
		case <-request.Context().Done():
			s.stop()
		case <-s.done:
			http.Error(response, "worker unavailable", 503)
		case result := <-events:
			var reply struct {
				ID     json.RawMessage `json:"id"`
				Result json.RawMessage `json:"result"`
			}
			if json.Unmarshal(result, &reply) != nil || string(reply.ID) != string(message.ID) || len(reply.Result) == 0 {
				s.stop()
				http.Error(response, "invalid initialization reply", 502)
				return
			}
			response.Header().Set("Content-Type", "application/json")
			response.Write(result)
		}
		return
	}
	w.stream(response, request, s, events, message.ID)
}
func (w *Worker) stream(response http.ResponseWriter, request *http.Request, s *session, events <-chan []byte, id json.RawMessage) {
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-store")
	controller := http.NewResponseController(response)
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	timer := time.NewTimer(65 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-request.Context().Done():
			if id != nil {
				s.stop()
			}
			return
		case <-s.done:
			return
		case <-timer.C:
			if id != nil {
				s.stop()
			}
			return
		case <-tick.C:
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprint(response, ": keepalive\n\n"); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
		case raw := <-events:
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprintf(response, "event: message\ndata: %s\n\n", raw); err != nil {
				if id != nil {
					s.stop()
				}
				return
			}
			if controller.Flush() != nil {
				return
			}
			var reply struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(raw, &reply) != nil {
				s.stop()
				return
			}
			if id != nil && reply.Method == "" && string(reply.ID) == string(id) {
				return
			}
		}
	}
}
