package config

import (
	"os"
	"time"
)

type Config struct {
	AppPort        string
	DatabaseURL    string
	JWTSecret      string
	JWTExpiresIn   time.Duration
	IdempotencyTTL time.Duration
}

func Load() *Config {
	return &Config{
		AppPort:        getEnv("APP_PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", ""),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		JWTExpiresIn:   getEnvDuration("JWT_EXPIRES_IN", 24*time.Hour),
		IdempotencyTTL: getEnvDuration("IDEMPOTENCY_TTL", 24*time.Hour),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
