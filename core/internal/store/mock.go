package store

import (
	"context"

	"github.com/bishoku/vmcp/core/internal/mockupstream"
)

// MockOwner is stable for this database and distinct across deployments even
// when both use the default tenant slug and share a WireMock instance.
func (t *Tx) MockOwner(ctx context.Context) (string, error) {
	var id string
	if err := t.q.QueryRow(ctx, `SELECT id::text FROM tenants WHERE id=`+tenantSQL).Scan(&id); err != nil {
		return "", err
	}
	return mockupstream.Owner(id), nil
}
