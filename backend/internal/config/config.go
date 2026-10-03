package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL      string
	TestDatabaseURL  string
	JWTSecret        string
	Port             string
	WorkerEnabled    bool
	AppEnv           string
	DBMaxConns       int32
	DBConnectTimeout time.Duration
}

func Load() (Config, error) {
	return load(10)
}

func LoadWorker() (Config, error) {
	return load(4)
}

func load(defaultMaxConns int32) (Config, error) {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv == "" || appEnv == "development" || appEnv == "dev" {
		_ = godotenv.Load()
	}

	appEnv = envOr("APP_ENV", "development")
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required; set it to your Supabase session-pooler URL")
	}

	maxConns, err := int32Env("DB_MAX_CONNS", defaultMaxConns)
	if err != nil {
		return Config{}, err
	}
	if maxConns < 1 {
		return Config{}, fmt.Errorf("DB_MAX_CONNS must be at least 1")
	}

	connectTimeout, err := durationEnv("DB_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	if connectTimeout <= 0 {
		return Config{}, fmt.Errorf("DB_CONNECT_TIMEOUT must be greater than zero")
	}

	workerEnabled, err := strconv.ParseBool(envOr("WORKER_ENABLED", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("WORKER_ENABLED must be true or false: %w", err)
	}

	port := envOr("PORT", "8080")
	if strings.ContainsAny(port, ":/ ") {
		return Config{}, fmt.Errorf("PORT must be a port number without a host or scheme")
	}

	return Config{
		DatabaseURL:      databaseURL,
		TestDatabaseURL:  strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		Port:             port,
		WorkerEnabled:    workerEnabled,
		AppEnv:           appEnv,
		DBMaxConns:       maxConns,
		DBConnectTimeout: connectTimeout,
	}, nil
}

func int32Env(name string, fallback int32) (int32, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return int32(parsed), nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a Go duration such as 5s: %w", name, err)
	}
	return parsed, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
