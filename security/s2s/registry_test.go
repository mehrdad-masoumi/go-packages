package s2s

import (
	"crypto/ed25519"
	"testing"
	"time"
)

func TestRegistryAuthenticate(t *testing.T) {
	reg := NewServiceRegistry([]ServiceCredential{
		{Name: "user-service", Secret: "secret", Audiences: []string{"auth-service"}, Scopes: []string{"auth.tokens.issue"}},
		{Name: " skip-me ", Secret: ""}, // ignored
	})
	if reg.Len() != 1 {
		t.Fatalf("len=%d", reg.Len())
	}
	cred, ok := reg.Authenticate("user-service", "secret")
	if !ok || cred.Name != "user-service" {
		t.Fatalf("authenticate failed: %#v ok=%v", cred, ok)
	}
	if _, ok := reg.Authenticate("user-service", "wrong"); ok {
		t.Fatal("expected secret rejection")
	}
	if !cred.MayRequestAudience("auth-service") {
		t.Fatal("expected auth-service audience allowed")
	}
	if cred.MayRequestAudience("wallet-service") {
		t.Fatal("expected wallet-service audience denied")
	}
}

func TestMayRequestAudienceEmptyAudiencesRejected(t *testing.T) {
	cred := ServiceCredential{Name: "orphan", Secret: "s", Audiences: nil}
	if cred.MayRequestAudience("user-service") {
		t.Fatal("empty audiences must be fail-closed")
	}
}

func TestMayRequestAudienceWildcard(t *testing.T) {
	cred := ServiceCredential{Name: "admin", Secret: "s", Audiences: []string{"*"}}
	if !cred.MayRequestAudience("any-service") {
		t.Fatal("explicit wildcard must allow any audience")
	}
}

func TestSignerIssue(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	signer, err := NewSigner(SignerConfig{Issuer: "auth", KeyID: "k1", PrivateKey: priv, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	tok, exp, err := signer.Issue("user-service", "auth-service", []string{"auth.tokens.issue"})
	if err != nil || tok == "" {
		t.Fatalf("issue: %v", err)
	}
	if exp.Before(time.Now()) {
		t.Fatal("expiry in the past")
	}
}
