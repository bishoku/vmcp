// Package secrets encrypts sensitive configuration and MCP replay at rest.
package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
)

var envelopeMagic = []byte{'V', 'M', 'C', 'P', 0, 1, 0, 1}
var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,63}$`)

// Box supports versioned, context-bound envelopes and legacy decrypt during a
// reader-first rolling upgrade. Keys are immutable after construction.
type Box struct {
	aead         cipher.AEAD
	keys         map[string]cipher.AEAD
	primary      string
	legacyWrites bool
	namespace    string
}

func NewBox(key string) (*Box, error) {
	return NewKeyring("primary", map[string]string{"primary": key}, false)
}
func NewKeyring(primary string, encoded map[string]string, legacyWrites bool) (*Box, error) {
	if !keyIDPattern.MatchString(primary) || len(encoded) == 0 || len(encoded) > 16 {
		return nil, errors.New("invalid encryption keyring")
	}
	b := &Box{primary: primary, keys: map[string]cipher.AEAD{}, legacyWrites: legacyWrites}
	for id, value := range encoded {
		if !keyIDPattern.MatchString(id) {
			return nil, errors.New("invalid encryption key ID")
		}
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != 32 {
			return nil, errors.New("encryption key must be base64-encoded 32 bytes")
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		b.keys[id] = aead
	}
	var ok bool
	b.aead, ok = b.keys[primary]
	if !ok {
		return nil, errors.New("primary encryption key is unavailable")
	}
	return b, nil
}

// FromEnv keeps VMCP_MASTER_KEY as the primary key. VMCP_DECRYPT_KEYS contains
// old key IDs and base64 values; it must never be logged.
func FromEnv(master string) (*Box, error) {
	primary := os.Getenv("VMCP_KEY_ID")
	if primary == "" {
		primary = "primary"
	}
	keys := map[string]string{}
	if raw := os.Getenv("VMCP_DECRYPT_KEYS"); raw != "" {
		if json.Unmarshal([]byte(raw), &keys) != nil || keys == nil {
			return nil, errors.New("VMCP_DECRYPT_KEYS must be a key-ID to base64-key JSON object")
		}
	}
	if previous, exists := keys[primary]; exists && previous != master {
		return nil, errors.New("primary key ID conflicts with decrypt keyring")
	}
	keys[primary] = master
	version := os.Getenv("VMCP_CIPHERTEXT_WRITE_VERSION")
	if version != "" && version != "v1" && version != "legacy" {
		return nil, errors.New("VMCP_CIPHERTEXT_WRITE_VERSION must be v1 or legacy")
	}
	box, err := NewKeyring(primary, keys, version == "legacy")
	if err != nil {
		return nil, err
	}
	tenant := os.Getenv("VMCP_TENANT_ID")
	if tenant != "" && tenant != "default" {
		box.namespace = tenant + "/"
	}
	return box, nil
}
func (b *Box) PrimaryID() string { return b.primary }
func (b *Box) KeyID(data []byte) (string, bool) {
	if !bytes.HasPrefix(data, envelopeMagic) || len(data) < len(envelopeMagic)+1 {
		return "", false
	}
	n := int(data[len(envelopeMagic)])
	start := len(envelopeMagic) + 1
	if n == 0 || n > 63 || len(data) < start+n {
		return "", false
	}
	id := string(data[start : start+n])
	return id, keyIDPattern.MatchString(id)
}
func aad(header []byte, entity, purpose string) ([]byte, error) {
	if len(entity) > 4096 || len(purpose) > 256 {
		return nil, errors.New("encryption context exceeds limit")
	}
	out := append([]byte(nil), header...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(entity)))
	out = append(out, entity...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(purpose)))
	return append(out, purpose...), nil
}
func (b *Box) SealBound(plaintext []byte, entity, purpose string) ([]byte, error) {
	if b.legacyWrites {
		nonce := make([]byte, b.aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return nil, err
		}
		return b.aead.Seal(nonce, nonce, plaintext, nil), nil
	}
	header := append(append([]byte(nil), envelopeMagic...), byte(len(b.primary)))
	header = append(header, b.primary...)
	context, err := aad(header, b.namespace+entity, purpose)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append(header, nonce...)
	return b.aead.Seal(out, nonce, plaintext, context), nil
}
func (b *Box) OpenBound(data []byte, entity, purpose string) ([]byte, error) {
	if !bytes.HasPrefix(data, envelopeMagic) {
		for _, aead := range b.keys {
			n := aead.NonceSize()
			if len(data) < n+aead.Overhead() {
				continue
			}
			if raw, err := aead.Open(nil, data[:n], data[n:], nil); err == nil {
				return raw, nil
			}
		}
		return nil, errors.New("legacy ciphertext authentication failed")
	}
	id, ok := b.KeyID(data)
	if !ok {
		return nil, errors.New("invalid ciphertext envelope")
	}
	aead, ok := b.keys[id]
	if !ok {
		return nil, errors.New("ciphertext key is unavailable")
	}
	headerLen := len(envelopeMagic) + 1 + len(id)
	n := aead.NonceSize()
	if len(data) < headerLen+n+aead.Overhead() {
		return nil, errors.New("ciphertext envelope too short")
	}
	context, err := aad(data[:headerLen], b.namespace+entity, purpose)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, data[headerLen:headerLen+n], data[headerLen+n:], context)
}
func (b *Box) Seal(raw []byte) ([]byte, error)  { return b.SealBound(raw, "", "generic") }
func (b *Box) Open(data []byte) ([]byte, error) { return b.OpenBound(data, "", "generic") }
func (b *Box) SealHeadersBound(h map[string]string, entity string) ([]byte, error) {
	if len(h) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	return b.SealBound(raw, entity, "upstream.headers")
}
func (b *Box) OpenHeadersBound(data []byte, entity string) (map[string]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	raw, err := b.OpenBound(data, entity, "upstream.headers")
	if err != nil {
		return nil, err
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return nil, fmt.Errorf("invalid decrypted headers: %w", err)
	}
	return headers, nil
}
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
func (b *Box) OpenHeaders(data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	raw, err := b.Open(data)
	if err != nil {
		return nil, err
	}
	var headers map[string]string
	err = json.Unmarshal(raw, &headers)
	return headers, err
}
