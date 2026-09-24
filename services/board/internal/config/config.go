// Package config loads board service settings from the environment.
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	GRPCAddr            string
	HTTPAddr            string
	MySQLDSN            string
	RedisAddr           string // event stream (Redis Streams consumer group)
	JWKSURL             string // where to FETCH keys (network address of the IdP)
	Issuer              string // expected iss claim (logical identifier, not the URL above)
	Audience            string // expected aud claim
	JWKSCacheTTL        time.Duration
	JWKSRefreshCooldown time.Duration
	JWKSBreakerTimeout  time.Duration
	DBMaxOpenConns      int
	DBMaxIdleConns      int
	DBConnMaxLifetime   time.Duration
}

func Load() Config {
	return Config{
		GRPCAddr:            envOr("GRPC_ADDR", ":9091"),
		HTTPAddr:            envOr("HTTP_ADDR", ":8091"),
		MySQLDSN:            envOr("MYSQL_DSN", "root:root@tcp(localhost:3306)/board?parseTime=true"),
		RedisAddr:           envOr("REDIS_ADDR", "localhost:6380"),
		JWKSURL:             envOr("JWKS_URL", "http://localhost:8090/oauth2/v1/jwks"),
		Issuer:              envOr("TOKEN_ISSUER", "http://localhost:8090"),
		Audience:            envOr("TOKEN_AUDIENCE", "board"),
		JWKSCacheTTL:        envDuration("JWKS_CACHE_TTL", 5*time.Minute),
		JWKSRefreshCooldown: envDuration("JWKS_REFRESH_COOLDOWN", 10*time.Second),
		JWKSBreakerTimeout:  envDuration("JWKS_BREAKER_TIMEOUT", 60*time.Second),
		DBMaxOpenConns:      envInt("DB_MAX_OPEN_CONNS", 40),
		DBMaxIdleConns:      envInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetime:   envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
	}
}

func envInt(key string, def int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
