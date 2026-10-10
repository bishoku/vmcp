package virtual

import (
	"os"
	"testing"
)

// Loopback fixtures are explicitly authorized only in test processes.
func TestMain(m *testing.M) {
	_ = os.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8,::1/128")
	_ = os.Setenv("VMCP_EGRESS_ALLOW_HOSTS", "")
	os.Exit(m.Run())
}
