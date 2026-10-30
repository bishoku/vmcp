package virtual

import (
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/correlation"
)

// Only the verified immutable agent-key ID selects a mapping. Metadata never selects its own profile.
func callerCorrelation(snap *config.Snapshot, owner Owner, meta json.RawMessage) correlation.IDs {
	if owner.Mode != "agent-key" {
		return correlation.IDs{}
	}
	for _, key := range snap.AgentKeys {
		if key.ID == owner.CredentialID {
			return key.AuditCorrelation.Extract(meta, owner.CredentialID)
		}
	}
	return correlation.IDs{}
}
