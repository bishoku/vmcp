package e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestPinnedTypeScriptSDKInterop(t *testing.T) {
	if os.Getenv("VMCP_REQUIRE_SDK_INTEROP") != "1" {
		return
	}
	e := setup(t)
	ctx, cancel := context.WithTimeout(e.ctx, 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "../../../web/scripts/mcp-interop.mjs", e.gwURL)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("SDK interoperability failed: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}
