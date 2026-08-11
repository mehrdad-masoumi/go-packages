package s2s

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type tokenSourceFunc func(context.Context, string) (string, error)

func (f tokenSourceFunc) Token(ctx context.Context, audience string) (string, error) {
	return f(ctx, audience)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestTransportInjectsBearerWithoutMutatingOriginal(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
	req.Header.Set("X-Test", "1")
	tr := Transport{
		Audience: "user-service",
		Source: tokenSourceFunc(func(_ context.Context, audience string) (string, error) {
			if audience != "user-service" {
				t.Fatalf("audience=%q", audience)
			}
			return "abc", nil
		}),
		Base: roundTripFunc(func(got *http.Request) (*http.Response, error) {
			if got.Header.Get("Authorization") != "Bearer abc" {
				t.Fatalf("authorization=%q", got.Header.Get("Authorization"))
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}),
	}
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("original request mutated")
	}
}
