package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// Server
	Port        string
	Environment string // "development" | "production"
	LogLevel    string

	// Database (Supabase PostgreSQL)
	DatabaseURL      string
	DatabaseMaxConns int
	DatabaseMinConns int

	// Supabase
	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseServiceKey string
	SupabaseJWTSecret  string

	// JWT
	JWTSecret    string
	JWTPublicKey string

	// CORS
	AllowedOrigins string

	// Redis (optional caching layer)
	RedisURL string

	// Domain restriction
	AllowedEmailDomain string
}

// Load reads configuration from environment variables.
// In development, it also reads from .env file.
func Load() (*Config, error) {
	// Load .env only in development; in production env vars are injected by Render
	if os.Getenv("ENVIRONMENT") != "production" {
		if err := godotenv.Load(); err != nil {
			// Not fatal – .env may not exist in CI
			fmt.Println("[config] .env file not found, using environment variables")
		}
	}

	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		Environment:        getEnv("ENVIRONMENT", "development"),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		DatabaseURL:        requireEnv("DATABASE_URL"),
		SupabaseURL:        requireEnv("SUPABASE_URL"),
		SupabaseAnonKey:    requireEnv("SUPABASE_ANON_KEY"),
		SupabaseServiceKey: requireEnv("SUPABASE_SERVICE_KEY"),
		SupabaseJWTSecret:  requireEnv("SUPABASE_JWT_SECRET"),
		JWTSecret:          requireEnv("SUPABASE_JWT_SECRET"),     // Same key
		JWTPublicKey:       requireEnv("SUPABASE_JWT_PUBLIC_KEY"), // New env for public key
		AllowedOrigins:     getEnv("ALLOWED_ORIGINS", "http://localhost:3000"),
		RedisURL:           getEnv("REDIS_URL", ""),
		AllowedEmailDomain: getEnv("ALLOWED_EMAIL_DOMAIN", "phantompestcontrol.ca"),
	}

	// Parse integer configs
	maxConns, _ := strconv.Atoi(getEnv("DB_MAX_CONNS", "20"))
	minConns, _ := strconv.Atoi(getEnv("DB_MIN_CONNS", "5"))
	cfg.DatabaseMaxConns = maxConns
	cfg.DatabaseMinConns = minConns

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("[config] required environment variable %q is not set", key))
	}
	return v
}
