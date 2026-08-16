package proxy

import (
	"net/http"
	"testing"
)

func TestClientIP_UntrustedPeerIgnoresForwarded(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "203.0.113.9:1234",
		Header:     http.Header{"X-Forwarded-For": []string{"10.0.0.1"}},
	}
	ip := ClientIP(r, Parse([]string{"10.0.0.0/8"}))
	if ip != "203.0.113.9" {
		t.Fatalf("spoofed XFF must be ignored, got %s", ip)
	}
}

func TestClientIP_TrustedProxyUsesForwarded(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "10.0.0.2:443",
		Header:     http.Header{"X-Forwarded-For": []string{"198.51.100.7, 10.0.0.2"}},
	}
	ip := ClientIP(r, Parse([]string{"10.0.0.0/8"}))
	if ip != "198.51.100.7" {
		t.Fatalf("trusted proxy should yield client IP, got %s", ip)
	}
}
