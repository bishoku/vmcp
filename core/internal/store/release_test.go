package store_test

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"sync"
	"testing"
)

func TestMigrationCoordinationAndSchemaGate(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if store.CheckSchema(ctx, pool) == nil {
		t.Fatal("unmigrated schema accepted")
	}
	var group sync.WaitGroup
	for i := 0; i < 3; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := store.Migrate(ctx, url); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if err := store.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
