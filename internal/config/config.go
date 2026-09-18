package config

import (
	"errors"
	"os"
	"time"
)

type Config struct {
	Addr        string
	DatabaseURL string
	Shutdown    time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	addr := getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	databaseURL := getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	return Config{Addr: addr, DatabaseURL: databaseURL, Shutdown: 10 * time.Second}, nil
}

func FromEnvironment() (Config, error) {
	return Load(os.Getenv)
}
