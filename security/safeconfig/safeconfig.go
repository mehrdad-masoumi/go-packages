// Package safeconfig prevents secret-bearing configuration from being logged
// or serialized. Services should log SafeSummary values, never raw config.
package safeconfig

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

var secretFieldTokens = []string{
	"password", "passwd", "secret", "token", "apikey", "api_key", "accesskey",
	"private", "encryption", "credential", "smtp", "minio", "mt5",
}

// Summary is a small, non-secret operational snapshot suitable for startup logs.
type Summary struct {
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
	Port        string `json:"port,omitempty"`
	DatabaseHost string `json:"database_host,omitempty"`
	RedisHost   string `json:"redis_host,omitempty"`
	RabbitHost  string `json:"rabbit_host,omitempty"`
	Features    map[string]any `json:"features,omitempty"`
}

func (s Summary) String() string {
	b, err := json.Marshal(s)
	if err != nil {
		return `{"error":"safeconfig_marshal"}`
	}
	return string(b)
}

// MarshalJSONRedacted marshals v after zeroing fields whose names look like secrets.
// Nested structs are walked. Use this only as a last-resort defensive dump.
func MarshalJSONRedacted(v any) ([]byte, error) {
	cloned := redactValue(reflect.ValueOf(v))
	if !cloned.IsValid() {
		return []byte("null"), nil
	}
	return json.Marshal(cloned.Interface())
}

func redactValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			fv := out.Field(i)
			if !fv.CanSet() {
				continue
			}
			if looksSecret(f.Name, f.Tag.Get("json"), f.Tag.Get("koanf")) {
				fv.Set(reflect.Zero(fv.Type()))
				continue
			}
			if fv.Kind() == reflect.Struct || fv.Kind() == reflect.Pointer || fv.Kind() == reflect.Slice || fv.Kind() == reflect.Map {
				red := redactValue(fv)
				if red.IsValid() && red.Type() == fv.Type() {
					fv.Set(red)
				}
			}
		}
		return out
	default:
		return v
	}
}

func looksSecret(name, jsonTag, koanfTag string) bool {
	blob := strings.ToLower(name + " " + jsonTag + " " + koanfTag)
	for _, tok := range secretFieldTokens {
		if strings.Contains(blob, tok) {
			return true
		}
	}
	return false
}

// ContainsSecret reports whether s includes any of the given secret values
// (used in tests to assert logs cannot leak configured credentials).
func ContainsSecret(s string, secrets ...string) bool {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(s, secret) {
			return true
		}
	}
	return false
}

func MustNotContainSecrets(s string, secrets ...string) error {
	if ContainsSecret(s, secrets...) {
		return fmt.Errorf("safeconfig: serialized output contains a secret value")
	}
	return nil
}
