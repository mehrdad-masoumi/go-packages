package metrics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNormalizePathReplacesIDs(t *testing.T) {
	got := HTTPOperation(http.MethodGet, "/users/84028a12-aaaa-bbbb-cccc-ddddeeeeffff")
	if got != "GET /users/:id" {
		t.Fatalf("got %q", got)
	}
	got = HTTPOperation(http.MethodPost, "/internal/broker/accounts/42/deposit")
	if got != "POST /internal/broker/accounts/:id/deposit" {
		t.Fatalf("got %q", got)
	}
}

func TestGRPCOperation(t *testing.T) {
	got := GRPCOperation("/user.v1.UserService/GetUser")
	if got != "UserService.GetUser" {
		t.Fatalf("got %q", got)
	}
}

func TestSanitizeRejectsHighCardinality(t *testing.T) {
	if SanitizeErrorType("token expired for user 12") != ErrorUnknown {
		t.Fatal("raw error text must not become a label")
	}
	if SanitizeEventType("wallet.credited.v1") != "wallet.credited.v1" {
		t.Fatal("bounded event type rejected")
	}
	if SanitizeEventType("/users/1?email=a@b.com") != "other" {
		t.Fatal("unbounded event type accepted")
	}
	if SanitizeBusinessOp("custom_op") != "" {
		t.Fatal("unknown business op must be dropped")
	}
}

func TestHTTPTransportRecords(t *testing.T) {
	ensure()
	rt := Transport{
		Source:      "wallet-service",
		Destination: "user-service",
		Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}),
	}
	req, _ := http.NewRequest(http.MethodGet, "http://user-service/users/99", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func TestErrorTypeFromContextCancel(t *testing.T) {
	if errorTypeFromErr(context.Canceled) != ErrorCanceled {
		t.Fatal("canceled")
	}
	if errorTypeFromErr(context.DeadlineExceeded) != ErrorTimeout {
		t.Fatal("timeout")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHTTPClientPreservesTimeout(t *testing.T) {
	base := &http.Client{Timeout: 3 * time.Second}
	got := HTTPClient(base, "wallet-service", "broker-service")
	if got.Timeout != 3*time.Second {
		t.Fatalf("timeout=%v", got.Timeout)
	}
}

func TestClassifyHTTP(t *testing.T) {
	st, et := HTTPStatusClass(200)
	if st != StatusSuccess || et != ErrorNone {
		t.Fatalf("%s %s", st, et)
	}
	st, et = HTTPStatusClass(401)
	if st != StatusError || et != ErrorUnauthenticated {
		t.Fatalf("%s %s", st, et)
	}
}

func TestIsInfraHTTPPath(t *testing.T) {
	if !IsInfraHTTPPath("/metrics") || !IsInfraHTTPPath("/health") || !IsInfraHTTPPath("/ready") {
		t.Fatal("infra paths")
	}
	if IsInfraHTTPPath("/api/users") {
		t.Fatal("api path treated as infra")
	}
}

func TestSanitizeService(t *testing.T) {
	if SanitizeService("wallet-service") != "wallet-service" {
		t.Fatal()
	}
	if SanitizeService("wallet service!!!") != "unknown" {
		t.Fatal()
	}
}
