package secrets

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func newKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

func TestRoundTrip(t *testing.T) {
	b, err := NewBox(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	h := map[string]string{"Authorization": "Bearer abc"}
	sealed, err := b.SealHeaders(h)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("Bearer abc")) {
		t.Fatal("plaintext leaked into ciphertext")
	}
	got, err := b.OpenHeaders(sealed)
	if err != nil || got["Authorization"] != "Bearer abc" {
		t.Fatalf("round trip failed: %v %v", got, err)
	}
}

func TestNonceIsRandom(t *testing.T) {
	b, _ := NewBox(newKey(t))
	a, _ := b.Seal([]byte("same"))
	c, _ := b.Seal([]byte("same"))
	if bytes.Equal(a, c) {
		t.Fatal("two seals of the same plaintext must differ")
	}
}

func TestTamperAndWrongKey(t *testing.T) {
	b1, _ := NewBox(newKey(t))
	b2, _ := NewBox(newKey(t))
	sealed, _ := b1.Seal([]byte("secret"))

	if _, err := b2.Open(sealed); err == nil {
		t.Fatal("wrong key must fail")
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := b1.Open(sealed); err == nil {
		t.Fatal("tampered data must fail")
	}
	if _, err := b1.Open([]byte("x")); err == nil {
		t.Fatal("short data must fail")
	}
}

func TestEmptyHeaders(t *testing.T) {
	b, _ := NewBox(newKey(t))
	if s, err := b.SealHeaders(nil); s != nil || err != nil {
		t.Fatalf("nil headers should seal to nil: %v %v", s, err)
	}
	if h, err := b.OpenHeaders(nil); h != nil || err != nil {
		t.Fatalf("nil should open to nil: %v %v", h, err)
	}
}

func TestBadKeys(t *testing.T) {
	if _, err := NewBox("not base64!!"); err == nil {
		t.Fatal("invalid base64 must fail")
	}
	if _, err := NewBox(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("wrong key length must fail")
	}
}
