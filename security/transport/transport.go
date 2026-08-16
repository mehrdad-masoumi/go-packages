// Package transport provides production-capable TLS helpers for gRPC and related
// internal traffic. Development may opt into plaintext; production must not
// silently fall back to insecure credentials.
package transport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type GRPCConfig struct {
	// TLS enables TLS. Required in production unless Insecure is explicitly true
	// AND the process is not production (Insecure is ignored in production).
	TLS bool
	// Insecure explicitly opts into plaintext. Ignored (rejected) when production is true.
	Insecure bool
	CAFile   string
	CertFile string
	KeyFile  string
	ServerName string
}

func GRPCDialOption(cfg GRPCConfig, production bool) (grpc.DialOption, error) {
	creds, err := GRPCClientCredentials(cfg, production)
	if err != nil {
		return nil, err
	}
	return grpc.WithTransportCredentials(creds), nil
}

func GRPCClientCredentials(cfg GRPCConfig, production bool) (credentials.TransportCredentials, error) {
	if production {
		if cfg.Insecure || !cfg.TLS {
			return nil, errors.New("grpc: insecure transport is not allowed in production")
		}
	}
	if cfg.Insecure || !cfg.TLS {
		return insecure.NewCredentials(), nil
	}
	tlsCfg, err := loadTLSConfig(cfg, false)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(tlsCfg), nil
}

func GRPCServerCredentials(cfg GRPCConfig, production bool) (credentials.TransportCredentials, error) {
	if production {
		if cfg.Insecure || !cfg.TLS {
			return nil, errors.New("grpc: insecure server transport is not allowed in production")
		}
	}
	if cfg.Insecure || !cfg.TLS {
		return insecure.NewCredentials(), nil
	}
	if strings.TrimSpace(cfg.CertFile) == "" || strings.TrimSpace(cfg.KeyFile) == "" {
		return nil, errors.New("grpc: tls cert_file and key_file are required")
	}
	tlsCfg, err := loadTLSConfig(cfg, true)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(tlsCfg), nil
}

func loadTLSConfig(cfg GRPCConfig, server bool) (*tls.Config, error) {
	out := &tls.Config{MinVersion: tls.VersionTLS12}
	if sn := strings.TrimSpace(cfg.ServerName); sn != "" {
		out.ServerName = sn
	}
	if ca := strings.TrimSpace(cfg.CAFile); ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("grpc: read ca_file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("grpc: failed to parse ca_file")
		}
		if server {
			out.ClientCAs = pool
			out.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			out.RootCAs = pool
		}
	}
	certFile, keyFile := strings.TrimSpace(cfg.CertFile), strings.TrimSpace(cfg.KeyFile)
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("grpc: load key pair: %w", err)
		}
		out.Certificates = []tls.Certificate{cert}
	}
	return out, nil
}

func ClientDialOption(tlsEnabled, insecure bool, caFile, serverName string, production bool) (grpc.DialOption, error) {
	return GRPCDialOption(GRPCConfig{
		TLS:        tlsEnabled,
		Insecure:   insecure,
		CAFile:     caFile,
		ServerName: serverName,
	}, production)
}

func AMQPScheme(tlsEnabled bool) string {
	if tlsEnabled {
		return "amqps"
	}
	return "amqp"
}

func RejectInsecureAMQP(tlsEnabled bool, production bool) error {
	if production && !tlsEnabled {
		return errors.New("rabbitmq: tls (amqps) is required in production")
	}
	return nil
}

func RejectInsecurePostgres(ssl string, production bool) error {
	if !production {
		return nil
	}
	s := strings.ToLower(strings.TrimSpace(ssl))
	if s == "" || s == "disable" {
		return errors.New("postgres: sslmode disable is not allowed in production")
	}
	return nil
}

func RejectInsecureSkipVerify(insecureSkipVerify, production bool) error {
	if production && insecureSkipVerify {
		return errors.New("tls: InsecureSkipVerify is not allowed in production")
	}
	return nil
}

func RejectInsecureRedis(tlsEnabled bool, production bool, host string) error {
	if !production {
		return nil
	}
	if AllowLoopbackSidecar(host) {
		return nil
	}
	if !tlsEnabled {
		return errors.New("redis: tls is required in production unless the endpoint is a local sidecar")
	}
	return nil
}

func RejectInsecureHTTPURL(raw string, production bool) error {
	if !production {
		return nil
	}
	u := strings.TrimSpace(raw)
	if u == "" {
		return errors.New("http: endpoint is required in production")
	}
	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "https://") {
		return nil
	}
	if strings.HasPrefix(lower, "http://") {
		host := hostFromURL(u)
		if AllowLoopbackSidecar(host) {
			return nil
		}
		return fmt.Errorf("http: plaintext endpoint %q is not allowed in production", host)
	}
	return errors.New("http: endpoint must be https in production")
}

func hostFromURL(raw string) string {
	without := raw
	for _, p := range []string{"https://", "http://"} {
		if strings.HasPrefix(strings.ToLower(without), p) {
			without = without[len(p):]
			break
		}
	}
	if i := strings.IndexAny(without, "/?"); i >= 0 {
		without = without[:i]
	}
	host, _, err := splitHostPort(without)
	if err != nil {
		return without
	}
	return host
}

func splitHostPort(hostport string) (string, string, error) {
	if !strings.Contains(hostport, ":") {
		return hostport, "", nil
	}
	return net.SplitHostPort(hostport)
}

func AllowLoopbackSidecar(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.Trim(h, "[]")
	switch h {
	case "", "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func RejectPublicPprof(enabled, production bool, bind string) error {
	if !enabled {
		return nil
	}
	if !production {
		return nil
	}
	b := strings.TrimSpace(bind)
	if b == "" || b == "0.0.0.0" || b == "::" || strings.HasPrefix(b, ":") {
		return errors.New("pprof: public bind is not allowed in production (disable or bind 127.0.0.1)")
	}
	if strings.HasPrefix(b, "127.0.0.1") || strings.HasPrefix(b, "localhost") {
		return nil
	}
	return errors.New("pprof: must bind loopback in production")
}

func ClientTLSConfig(caFile, serverName string, insecureSkipVerify bool) (*tls.Config, error) {
	if insecureSkipVerify {
		return nil, errors.New("tls: InsecureSkipVerify callers must use ClientTLSConfigDev")
	}
	return clientTLSConfig(caFile, serverName, false)
}

func ClientTLSConfigDev(caFile, serverName string, insecureSkipVerify bool) (*tls.Config, error) {
	return clientTLSConfig(caFile, serverName, insecureSkipVerify)
}

func clientTLSConfig(caFile, serverName string, insecureSkipVerify bool) (*tls.Config, error) {
	out := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecureSkipVerify}
	if sn := strings.TrimSpace(serverName); sn != "" {
		out.ServerName = sn
	}
	if ca := strings.TrimSpace(caFile); ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			return nil, err
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("tls: failed to parse ca_file")
		}
		out.RootCAs = pool
	}
	return out, nil
}
