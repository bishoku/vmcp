package delivery

import (
	"context"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"testing"
)

func TestDeliveryAdmissionHasNoDistributedCapacityGate(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &Store{Pool: pool}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version) VALUES('s','v','','2025-06-18')`); err != nil {
		t.Fatal(err)
	}
	// Holding the old lock must not serialize new admissions. The former principal
	// threshold must not reject requests admitted by independent pods.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(19840923,hashtext('v'))`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if _, err = s.Begin(ctx, "s", fmt.Sprint(i), "v", []byte("principal")); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err = s.Begin(ctx, "s", "0", "v", []byte("principal")); err == nil {
		t.Fatal("duplicate live request accepted")
	}
}
