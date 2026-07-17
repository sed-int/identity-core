// Package config loads identity service settings from the environment.
package config

import "os"

type Config struct {
	GRPCAddr       string
	HTTPAddr       string
	MySQLDSN       string
	RedisAddr      string
	Issuer         string // iss claim + base URL in the discovery document
	SigningKeyPath string
	DevMode        bool // echoes OTP codes in responses; never enable outside local dev
}

func Load() Config {
	return Config{
		GRPCAddr:       envOr("GRPC_ADDR", ":9090"),
		HTTPAddr:       envOr("HTTP_ADDR", ":8090"),
		MySQLDSN:       envOr("MYSQL_DSN", "root:root@tcp(localhost:3306)/identity?parseTime=true"),
		RedisAddr:      envOr("REDIS_ADDR", "localhost:6380"),
		Issuer:         envOr("ISSUER", "http://localhost:8090"),
		SigningKeyPath: envOr("SIGNING_KEY_PATH", "data/keys/identity-signing.pem"),
		DevMode:        envOr("DEV_MODE", "true") == "true",
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
