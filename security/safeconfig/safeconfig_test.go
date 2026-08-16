package safeconfig

import "testing"

type sampleCfg struct {
	Host     string `json:"host" koanf:"host"`
	Password string `json:"-" koanf:"password"`
	Redis    struct {
		Host     string `json:"host"`
		Password string `json:"password"`
	} `json:"redis"`
	JWTSecret string `koanf:"jwt_secret"`
}

func TestMarshalJSONRedactedStripsSecrets(t *testing.T) {
	cfg := sampleCfg{Host: "db.internal", Password: "super-secret-db", JWTSecret: "jwt-secret-value"}
	cfg.Redis.Host = "redis.internal"
	cfg.Redis.Password = "redis-secret-value"
	b, err := MarshalJSONRedacted(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if err := MustNotContainSecrets(out, "super-secret-db", "redis-secret-value", "jwt-secret-value"); err != nil {
		t.Fatal(err, out)
	}
	if !ContainsSecret(out, "db.internal") || !ContainsSecret(out, "redis.internal") {
		t.Fatalf("expected hosts to remain: %s", out)
	}
}

func TestSummaryNeverIncludesCredentials(t *testing.T) {
	s := Summary{Service: "broker-service", Environment: "production", Port: "8087", DatabaseHost: "pg", RedisHost: "redis", RabbitHost: "mq"}
	if err := MustNotContainSecrets(s.String(), "password", "secret"); err != nil {
		t.Fatal(err)
	}
}
