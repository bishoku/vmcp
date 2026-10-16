// Package testdb gives each test its own throwaway Postgres database, so packages
// can run in parallel without seeing each other's data.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// New creates an empty database on the server named by VMCP_TEST_DATABASE_URL and
// returns its URL. The test is skipped when the variable is not set. The database
// is dropped when the test finishes (after any pool opened by the test is closed,
// as cleanups run last-in-first-out).
func New(t testing.TB) string {
	t.Helper()
	base := os.Getenv("VMCP_TEST_DATABASE_URL")
	if base == "" {
		if os.Getenv("VMCP_REQUIRE_INTEGRATION") == "1" {
			t.Fatal("required PostgreSQL integration URL is missing")
		}
		t.Skip("VMCP_TEST_DATABASE_URL not set")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse VMCP_TEST_DATABASE_URL: %v", err)
	}

	var b [6]byte
	_, _ = rand.Read(b[:])
	name := "vmcp_t_" + hex.EncodeToString(b[:])

	admin := *u
	admin.Path = "/postgres"
	exec := func(sql string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, admin.String())
		if err != nil {
			return err
		}
		defer conn.Close(ctx)
		_, err = conn.Exec(ctx, sql)
		return err
	}
	if err := exec("CREATE DATABASE " + name); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		if err := exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"); err != nil {
			t.Logf("drop test database %s: %v", name, err)
		}
	})

	u.Path = "/" + name
	return u.String()
}
