// Package config loads board service settings from the environment.
package config

import "os"

type Config struct {
	GRPCAddr  string
	HTTPAddr  string
	MySQLDSN  string
	RedisAddr string // event stream (Redis Streams consumer group)
	JWKSURL  string // where to FETCH keys (network address of the IdP)
	Issuer   string // expected iss claim (logical identifier, not the URL above)
	Audience string // expected aud claim
}

func Load() Config {
	return Config{
		GRPCAddr:  envOr("GRPC_ADDR", ":9091"),
		HTTPAddr:  envOr("HTTP_ADDR", ":8091"),
		MySQLDSN:  envOr("MYSQL_DSN", "root:root@tcp(localhost:3306)/board?parseTime=true"),
		RedisAddr: envOr("REDIS_ADDR", "localhost:6380"),
		JWKSURL:  envOr("JWKS_URL", "http://localhost:8090/oauth2/v1/jwks"),
		Issuer:   envOr("TOKEN_ISSUER", "http://localhost:8090"),
		Audience: envOr("TOKEN_AUDIENCE", "board"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
