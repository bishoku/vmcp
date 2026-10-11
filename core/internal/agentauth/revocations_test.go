package agentauth_test

import (
	"context"
	"crypto/sha256"
	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"testing"
	"time"
)

func TestSharedTokenDenylist(t *testing.T) {
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
	a, b := agentauth.PostgresRevocations{Pool: pool}, agentauth.PostgresRevocations{Pool: pool}
	hash := sha256.Sum256([]byte("exact-token"))
	if revoked, err := a.Revoked(ctx, hash[:]); err != nil || revoked {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO revoked_jwts VALUES($1,$2)`, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if revoked, err := b.Revoked(ctx, hash[:]); err != nil || !revoked {
		t.Fatal("revocation not shared", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE revoked_jwts SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if revoked, err := b.Revoked(ctx, hash[:]); err != nil || revoked {
		t.Fatal("expired denylist entry", err)
	}
}
