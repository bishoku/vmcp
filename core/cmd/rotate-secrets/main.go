// rotate-secrets re-encrypts stored values with the configured primary key.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "secret rotation failed; restore key availability and rerun")
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	box, err := secrets.FromEnv(os.Getenv("VMCP_MASTER_KEY"))
	if err != nil {
		return err
	}
	if os.Getenv("VMCP_CIPHERTEXT_WRITE_VERSION") == "legacy" {
		return fmt.Errorf("rotation requires v1 writers")
	}
	pool, err := pgxpool.New(ctx, os.Getenv("VMCP_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.CheckTenant(ctx, pool); err != nil {
		return err
	}
	// Identifiers are compile-time constants, never administrator input. Each
	// committed row is its own checkpoint; CAS preserves concurrent edits.
	fields := []struct{ table, column, purpose string }{
		{"upstreams", "encrypted_headers", "upstream.headers"},
		{"upstreams", "encrypted_client_key", "upstream.client_key"},
		{"upstreams", "encrypted_oauth_secret", "upstream.oauth_secret"},
		{"mcp_events", "payload", "mcp.event"},
		{"mcp_pending_requests", "response", "mcp.response"},
	}
	for _, field := range fields {
		cursor := "00000000-0000-0000-0000-000000000000"
		for {
			bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
			rows, err := pool.Query(bounded, "SELECT id::text,"+field.column+" FROM "+field.table+" WHERE id>$1::uuid AND "+field.column+" IS NOT NULL ORDER BY id LIMIT 100", cursor)
			if err != nil {
				cancel()
				return err
			}
			type record struct {
				id   string
				data []byte
			}
			batch := []record{}
			for rows.Next() {
				var r record
				if err := rows.Scan(&r.id, &r.data); err != nil {
					rows.Close()
					cancel()
					return err
				}
				batch = append(batch, r)
			}
			err = rows.Err()
			rows.Close()
			cancel()
			if err != nil {
				return err
			}
			if len(batch) == 0 {
				break
			}
			for _, r := range batch {
				cursor = r.id
				if len(r.data) == 0 {
					continue
				}
				raw, err := box.OpenBound(r.data, r.id, field.purpose)
				if err != nil {
					return err
				}
				if id, ok := box.KeyID(r.data); ok && id == box.PrimaryID() {
					clear(raw)
					continue
				}
				replacement, err := box.SealBound(raw, r.id, field.purpose)
				clear(raw)
				if err != nil {
					return err
				}
				bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, err = pool.Exec(bounded, "UPDATE "+field.table+" SET "+field.column+"=$1 WHERE id=$2 AND "+field.column+"=$3", replacement, r.id, r.data)
				cancel()
				if err != nil {
					return err
				}
			}
		}
	}
	fmt.Println("secret rotation completed; retain recovery keys until restore verification")
	return nil
}
