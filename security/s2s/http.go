package s2s

import (
	"context"
	"errors"
	"net/http"
)

type TokenSource interface {
	Token(ctx context.Context, audience string) (string, error)
}

// Transport injects a destination-scoped service token into outbound HTTP requests.
// It clones the request before mutating headers.
type Transport struct {
	Base     http.RoundTripper
	Source   TokenSource
	Audience string
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("s2s transport: request is nil")
	}
	if t.Source == nil {
		return nil, errors.New("s2s transport: token source is required")
	}
	if t.Audience == "" {
		return nil, errors.New("s2s transport: audience is required")
	}
	token, err := t.Source.Token(req.Context(), t.Audience)
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", AuthorizationHeader(token))
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}
