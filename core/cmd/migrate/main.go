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
}
