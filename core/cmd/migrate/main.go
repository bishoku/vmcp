// migrate is the deployment-only schema migration entry point.
package main

import (
	"context"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/store"
	"os"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := store.Migrate(ctx, os.Getenv("VMCP_DATABASE_URL")); err != nil {
		fmt.Fprintln(os.Stderr, "database migration failed")
		os.Exit(1)
	}
	pool, err := store.Connect(ctx, os.Getenv("VMCP_DATABASE_URL"))
	if err != nil {
		os.Exit(1)
	}
	defer pool.Close()
	if err := store.BindTenant(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, "tenant database binding failed")
		os.Exit(1)
	}
}
