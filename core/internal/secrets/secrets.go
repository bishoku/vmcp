// Package secrets encrypts sensitive config values (upstream auth headers) at rest.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// Box seals and opens values with AES-256-GCM. Output layout: nonce || ciphertext.
type Box struct {
	aead cipher.AEAD
}

// NewBox builds a Box from a base64-encoded 32-byte key.
func NewBox(base64Key string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("master key is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext with a fresh random nonce.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts data produced by Seal.
func (b *Box) Open(data []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(data) < n+b.aead.Overhead() {
		return nil, errors.New("ciphertext too short")
	}
	return b.aead.Open(nil, data[:n], data[n:], nil)
}

// SealHeaders encrypts a header map. A nil or empty map yields nil.
func (b *Box) SealHeaders(h map[string]string) ([]byte, error) {
	if len(h) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	return b.Seal(raw)
}

// OpenHeaders decrypts a header map produced by SealHeaders.
func (b *Box) OpenHeaders(data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	raw, err := b.Open(data)
	if err != nil {
		return nil, err
	}
	var h map[string]string
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, err
	}
	return h, nil
}
