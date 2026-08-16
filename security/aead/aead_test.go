package aead

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := New(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := c.Encrypt("mt5-secret")
	if err != nil {
		t.Fatal(err)
	}
	if ct == "mt5-secret" || !IsEnvelope(ct) {
		t.Fatalf("expected versioned envelope, got %q", ct)
	}
	got, err := c.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != "mt5-secret" {
		t.Fatalf("got %q", got)
	}
}

func TestDecryptLegacyPlaintextUnchanged(t *testing.T) {
	c, err := New(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Decrypt("already-plaintext")
	if err != nil {
		t.Fatal(err)
	}
	if got != "already-plaintext" {
		t.Fatalf("got %q", got)
	}
}

func TestRejectsEmptyKey(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestTamperRejected(t *testing.T) {
	c, err := New(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := c.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	tampered := ct[:len(ct)-2] + "aa"
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("expected auth failure")
	}
}
