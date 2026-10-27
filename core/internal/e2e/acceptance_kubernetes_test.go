//go:build acceptance

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func acceptanceKubernetes() bool { return os.Getenv("VMCP_ACCEPTANCE_KUBERNETES") == "1" }
func acceptanceKubectl(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	namespace := os.Getenv("VMCP_ACCEPTANCE_NAMESPACE")
	if !strings.HasPrefix(namespace, "vmcp-load-") {
		return nil, fmt.Errorf("load tests require their own vmcp-load-* namespace")
	}
	command := exec.CommandContext(ctx, "microk8s", append([]string{"kubectl", "--namespace", namespace}, args...)...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("test namespace Kubernetes command failed: %w", err)
	}
	return output, nil
}
func (n *acceptanceNode) startKubernetes(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := url.Parse(n.database)
	if err != nil {
		t.Fatal(err)
	}
	database.Host = "postgres:5432"
	secret := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]string{"name": "runtime"}, "stringData": map[string]string{"database-url": database.String(), "master-key": n.master}}
	raw, _ := json.Marshal(secret)
	if _, err = acceptanceKubectl(ctx, raw, "apply", "-f", "-"); err != nil {
		t.Fatal(err)
	}
	if _, err = acceptanceKubectl(ctx, nil, "scale", "deployment/control-plane", "--replicas=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = acceptanceKubectl(ctx, nil, "scale", "deployment/"+n.name, "--replicas=1"); err != nil {
		t.Fatal(err)
	}
	raw, err = acceptanceKubectl(ctx, nil, "get", "service", n.name, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var service struct {
		Spec struct{ Ports []struct{ NodePort int } }
	}
	if json.Unmarshal(raw, &service) != nil || len(service.Spec.Ports) != 1 {
		t.Fatal("missing load-test node port")
	}
	n.mu.Lock()
	n.url = fmt.Sprintf("http://%s:%d", os.Getenv("VMCP_ACCEPTANCE_NODE_IP"), service.Spec.Ports[0].NodePort)
	n.alive = true
	n.mu.Unlock()
	if os.Getenv("VMCP_ACCEPTANCE_ROUTING") == "pod" {
		if _, err := acceptanceKubectl(ctx, nil, "rollout", "status", "deployment/"+n.name, "--timeout=110s"); err != nil {
			t.Fatal(err)
		}
		raw, err := acceptanceKubectl(ctx, nil, "get", "pods", "-l", "app="+n.name, "-o", "json")
		if err != nil {
			t.Fatal(err)
		}
		var pods struct {
			Items []struct{ Status struct{ PodIP string } }
		}
		if json.Unmarshal(raw, &pods) != nil || len(pods.Items) != 1 || pods.Items[0].Status.PodIP == "" {
			t.Fatal("expected one ready test pod")
		}
		n.mu.Lock()
		n.url = "http://" + net.JoinHostPort(pods.Items[0].Status.PodIP, "8080")
		n.mu.Unlock()
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for ctx.Err() == nil {
		response, err := client.Get(n.url + "/readyz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				t.Logf("%s Kubernetes pod ready at dedicated test Service", n.name)
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("Kubernetes data-plane did not become ready")
}
func (n *acceptanceNode) killKubernetes() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tail := "--tail=3000"
	if os.Getenv("VMCP_ACCEPTANCE_DIAGNOSTICS") == "1" {
		tail = "--tail=-1"
	}
	if logs, err := acceptanceKubectl(ctx, nil, "logs", "deployment/"+n.name, "--all-containers=true", tail); err == nil {
		if file, err := os.OpenFile(filepath.Join(n.output, n.name+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
			_, _ = file.Write(logs)
			_ = file.Close()
		}
	}
	if _, err := acceptanceKubectl(ctx, nil, "scale", "deployment/"+n.name, "--replicas=0"); err != nil {
		n.mu.Lock()
		n.kubeError = err.Error()
		n.mu.Unlock()
		return
	}
	n.mu.Lock()
	n.alive = false
	n.mu.Unlock()
}

// Kubelet container RSS and recent CPU usage, rather than the host driver PID.
// Summary data is read-only; only the dedicated test namespace is retained.
func acceptanceKubeSamples(ctx context.Context) map[string]acceptanceNodeSample {
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := acceptanceKubectl(bounded, nil, "get", "--raw", "/api/v1/nodes/"+os.Getenv("VMCP_ACCEPTANCE_KUBE_NODE")+"/proxy/stats/summary")
	result := map[string]acceptanceNodeSample{}
	if err != nil {
		return result
	}
	var summary struct {
		Pods []struct {
			PodRef     struct{ Namespace, Name string }
			Containers []struct {
				Name   string
				Memory struct{ RSSBytes, UsageBytes int64 }
				CPU    struct{ UsageNanoCores uint64 }
			}
		}
	}
	if json.Unmarshal(raw, &summary) != nil {
		return result
	}
	for _, pod := range summary.Pods {
		if pod.PodRef.Namespace != os.Getenv("VMCP_ACCEPTANCE_NAMESPACE") {
			continue
		}
		for _, container := range pod.Containers {
			if container.Name != "data-plane" {
				result[container.Name] = acceptanceNodeSample{Name: container.Name, Alive: true, RSSBytes: container.Memory.RSSBytes, ReportedCPUPercent: float64(container.CPU.UsageNanoCores) / 1e7}
				continue
			}
			for _, name := range []string{"acceptance-node-1", "acceptance-node-2"} {
				if strings.HasPrefix(pod.PodRef.Name, name+"-") {
					previous := result[name]
					previous.Name = name
					previous.Alive = true
					previous.RSSBytes += container.Memory.RSSBytes
					previous.MemoryUsageBytes += container.Memory.UsageBytes
					previous.ReportedCPUPercent += float64(container.CPU.UsageNanoCores) / 1e7
					result[name] = previous
				}
			}
		}
	}
	return result
}

func TestAcceptanceKubeNamespaceGuard(t *testing.T) {
	t.Setenv("VMCP_ACCEPTANCE_NAMESPACE", "vmcp-demo")
	if _, err := acceptanceKubectl(context.Background(), nil, "scale", "deployment/acceptance-node-1", "--replicas=0"); err == nil {
		t.Fatal("existing namespace was accepted")
	}
}
