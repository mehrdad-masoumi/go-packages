package transport

import "testing"

func TestGRPCClientCredentials_ProductionRejectsInsecure(t *testing.T) {
	_, err := GRPCClientCredentials(GRPCConfig{Insecure: true}, true)
	if err == nil {
		t.Fatal("expected production to reject insecure gRPC")
	}
	_, err = GRPCClientCredentials(GRPCConfig{}, true)
	if err == nil {
		t.Fatal("expected production to reject plaintext gRPC")
	}
}

func TestRejectInsecureAMQPAndPostgres(t *testing.T) {
	if err := RejectInsecureAMQP(false, true); err == nil {
		t.Fatal("expected production AMQP plaintext rejected")
	}
	if err := RejectInsecureAMQP(true, true); err != nil {
		t.Fatal(err)
	}
	if err := RejectInsecurePostgres("disable", true); err == nil {
		t.Fatal("expected production postgres ssl disable rejected")
	}
	if err := RejectInsecurePostgres("require", true); err != nil {
		t.Fatal(err)
	}
	if got := AMQPScheme(true); got != "amqps" {
		t.Fatalf("got %s", got)
	}
	if got := AMQPScheme(false); got != "amqp" {
		t.Fatalf("got %s", got)
	}
}

func TestRejectInsecureHTTPURL(t *testing.T) {
	if err := RejectInsecureHTTPURL("http://wallet-api:8080", true); err == nil {
		t.Fatal("expected production plaintext http rejected")
	}
	if err := RejectInsecureHTTPURL("https://wallet-api:8080", true); err != nil {
		t.Fatal(err)
	}
	if err := RejectInsecureHTTPURL("http://127.0.0.1:5005", true); err != nil {
		t.Fatal("loopback sidecar should be allowed", err)
	}
	if err := RejectInsecureHTTPURL("http://localhost:5005/api", true); err != nil {
		t.Fatal(err)
	}
}

func TestRejectPublicPprof(t *testing.T) {
	if err := RejectPublicPprof(true, true, ":6060"); err == nil {
		t.Fatal("expected public pprof rejected")
	}
	if err := RejectPublicPprof(true, true, "127.0.0.1:6060"); err != nil {
		t.Fatal(err)
	}
	if err := RejectPublicPprof(false, true, ":6060"); err != nil {
		t.Fatal(err)
	}
}
