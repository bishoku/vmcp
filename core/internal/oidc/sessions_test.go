package oidc

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionRevocationAcrossNodes(t *testing.T) {
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
	db := PostgresSessions{Pool: pool}
	session := Session{ID: random(), Subject: "account", Role: RoleAdmin, Expires: time.Now().Add(time.Hour)}
	if err := db.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	a := &Provider{cfg: Config{SessionKey: make([]byte, 32), Sessions: db}, now: time.Now}
	b := &Provider{cfg: a.cfg, now: time.Now}
	w := httptest.NewRecorder()
	a.setCookie(w, sessionCookie, session, time.Hour)
	request := httptest.NewRequest("GET", "/", nil)
	request.AddCookie(w.Result().Cookies()[0])
	if _, ok := b.Session(request); !ok {
		t.Fatal("shared session rejected")
	}
	if !a.Logout(httptest.NewRecorder(), request) {
		t.Fatal("logout failed")
	}
	if _, ok := b.Session(request); ok {
		t.Fatal("revoked cookie accepted on another node")
	}
	session.ID = random()
	if err := db.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeSubject(ctx, "account"); err != nil {
		t.Fatal(err)
	}
	if valid, err := db.Valid(ctx, session.ID); err != nil || valid {
		t.Fatal("account revocation failed", err)
	}
}
