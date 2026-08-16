// Package proxy extracts client IPs using a trusted-proxy allowlist.
// Forwarded headers are ignored unless the immediate peer is trusted.
package proxy

import (
	"net"
	"net/http"
	"strings"
)

type Config struct {
	// CIDRs or exact IPs of reverse proxies (e.g. Traefik) allowed to set
	// X-Forwarded-For / X-Real-IP. Empty means never trust forwarded headers.
	TrustedProxies []string
}

func Parse(trusted []string) []*net.IPNet {
	var out []*net.IPNet
	for _, raw := range trusted {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			if ip := net.ParseIP(raw); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				raw = ip.String() + "/" + itoa(bits)
			}
		}
		_, n, err := net.ParseCIDR(raw)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func itoa(n int) string {
	if n == 32 {
		return "32"
	}
	if n == 128 {
		return "128"
	}
	return "0"
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isTrusted(ip string, nets []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// ClientIP returns the remote IP. X-Forwarded-For / X-Real-IP are used only
// when the immediate peer is in the trusted proxy set.
func ClientIP(r *http.Request, trusted []*net.IPNet) string {
	peer := peerIP(r)
	if peer == "" {
		peer = "unknown"
	}
	if !isTrusted(peer, trusted) {
		return peer
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		candidate := strings.TrimSpace(parts[0])
		if net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}
	return peer
}
