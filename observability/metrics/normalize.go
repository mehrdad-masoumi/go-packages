package metrics

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var (
	uuidRE      = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	hexIDRE     = regexp.MustCompile(`(?i)^[0-9a-f]{16,}$`)
	digitsRE    = regexp.MustCompile(`^[0-9]{1,20}$`)
	eventTypeRE = regexp.MustCompile(`^[a-z][a-z0-9._]{0,62}$`)
	serviceRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

var allowedErrorTypes = map[string]struct{}{
	ErrorNone:            {},
	ErrorUnauthenticated: {},
	ErrorForbidden:       {},
	ErrorNotFound:        {},
	ErrorInvalid:         {},
	ErrorConflict:        {},
	ErrorRateLimited:     {},
	ErrorTimeout:         {},
	ErrorCanceled:        {},
	ErrorUnavailable:     {},
	ErrorServer:          {},
	ErrorUnknown:         {},
}

var allowedReasons = map[string]struct{}{
	ReasonNone:             {},
	ReasonMissingToken:     {},
	ReasonExpired:          {},
	ReasonInvalidSignature: {},
	ReasonInvalidAudience:  {},
	ReasonInvalidIssuer:    {},
	ReasonMissingScope:     {},
	ReasonUnknownService:   {},
	ReasonTokenFetchFailed: {},
}

var allowedResults = map[string]struct{}{
	ResultSuccess:    {},
	ResultError:      {},
	ResultRetry:      {},
	ResultDLQ:        {},
	ResultAbandoned:  {},
	ResultTimeout:    {},
	ResultCanceled:   {},
	ResultClaimed:    {},
	ResultDispatched: {},
	ResultFailed:     {},
	ResultRetried:    {},
}

var allowedBusinessOps = map[string]struct{}{
	OpWalletCredit:       {},
	OpWalletDebit:        {},
	OpDepositComplete:    {},
	OpWithdrawalComplete: {},
	OpTradeOpened:        {},
	OpTradeClosed:        {},
	OpKYCApproved:        {},
	OpKYCRejected:        {},
	OpTicketCreated:      {},
}

func SanitizeService(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || !serviceRE.MatchString(name) {
		return "unknown"
	}
	return name
}

func SanitizeErrorType(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if _, ok := allowedErrorTypes[v]; ok {
		return v
	}
	return ErrorUnknown
}

func SanitizeReason(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if _, ok := allowedReasons[v]; ok {
		return v
	}
	return ReasonInvalidSignature
}

func SanitizeResult(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if _, ok := allowedResults[v]; ok {
		return v
	}
	return ResultError
}

func SanitizeBusinessOp(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if _, ok := allowedBusinessOps[v]; ok {
		return v
	}
	return ""
}

func SanitizeEventType(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if eventTypeRE.MatchString(v) && !strings.Contains(v, "..") {
		return v
	}
	return "other"
}

func SanitizeExchange(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range v {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" || len(out) > 64 {
		return "other"
	}
	return out
}

func SanitizeOperation(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	if len(v) > 128 {
		v = v[:128]
	}
	return v
}

// HTTPOperation returns a stable logical operation such as "GET /users/:id".
func HTTPOperation(method, path string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "GET"
	}
	return method + " " + NormalizePath(path)
}

// NormalizePath replaces IDs/UUIDs with :id so Prometheus labels stay bounded.
func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if u, err := url.Parse(path); err == nil {
		if u.Path != "" {
			path = u.Path
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if uuidRE.MatchString(p) || hexIDRE.MatchString(p) || digitsRE.MatchString(p) {
			parts[i] = ":id"
		}
	}
	out := strings.Join(parts, "/")
	if out == "" {
		return "/"
	}
	if len(out) > 96 {
		return out[:96]
	}
	return out
}

// GRPCOperation converts "/pkg.UserService/GetUser" to "UserService.GetUser".
func GRPCOperation(fullMethod string) string {
	fullMethod = strings.TrimSpace(fullMethod)
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	if fullMethod == "" {
		return "unknown"
	}
	svc, method, ok := strings.Cut(fullMethod, "/")
	if !ok {
		return SanitizeOperation(fullMethod)
	}
	if i := strings.LastIndex(svc, "."); i >= 0 {
		svc = svc[i+1:]
	}
	return SanitizeOperation(svc + "." + method)
}

func HTTPStatusClass(code int) (status, errorType string) {
	switch {
	case code >= 200 && code < 400:
		return StatusSuccess, ErrorNone
	case code == http.StatusUnauthorized:
		return StatusError, ErrorUnauthenticated
	case code == http.StatusForbidden:
		return StatusError, ErrorForbidden
	case code == http.StatusNotFound:
		return StatusError, ErrorNotFound
	case code == http.StatusConflict:
		return StatusError, ErrorConflict
	case code == http.StatusTooManyRequests:
		return StatusError, ErrorRateLimited
	case code >= 400 && code < 500:
		return StatusError, ErrorInvalid
	case code >= 500:
		return StatusError, ErrorServer
	default:
		return StatusError, ErrorUnknown
	}
}

func IsInfraHTTPPath(path string) bool {
	p := strings.ToLower(NormalizePath(path))
	switch p {
	case "/metrics", "/health", "/health-check", "/ready", "/ping", "/live", "/liveness", "/startup":
		return true
	default:
		return false
	}
}
