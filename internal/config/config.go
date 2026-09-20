package config

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                 string
	DatabaseURL          string
	MigrationDatabaseURL string
	Shutdown             time.Duration
	// AppBaseURL is the origin a reset link points at. It is the public URL of
	// the web app, not the API: the operator clicks the link in a browser.
	AppBaseURL string
	SMTP       SMTPConfig
	// MasterKey seals provider credentials (US-AD86, ARCHITECTURE §16). It is
	// carried as the raw string and decoded by internal/crypto on use, so an
	// empty or malformed value does not stop the API from booting: only the
	// credential endpoints need it, and refusing to start would take the whole
	// product down over a feature nobody has used yet.
	MasterKey string
}

// SMTPConfig is the relay the API sends through. An empty Host selects the
// logging mailer, which is the correct default for a self-hosted install that
// has not configured mail yet.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
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
	migrationDatabaseURL := getenv("MIGRATION_DATABASE_URL")
	if migrationDatabaseURL == "" {
		migrationDatabaseURL = databaseURL
	}
	appBaseURL := strings.TrimRight(getenv("APP_BASE_URL"), "/")
	if appBaseURL == "" {
		appBaseURL = "http://localhost:5173"
	}
	return Config{
		Addr:                 addr,
		DatabaseURL:          databaseURL,
		MigrationDatabaseURL: migrationDatabaseURL,
		Shutdown:             10 * time.Second,
		AppBaseURL:           appBaseURL,
		MasterKey:            getenv("AGENTDECK_MASTER_KEY"),
		SMTP: SMTPConfig{
			Host:     getenv("SMTP_HOST"),
			Port:     getenv("SMTP_PORT"),
			Username: getenv("SMTP_USERNAME"),
			Password: getenv("SMTP_PASSWORD"),
			From:     getenv("SMTP_FROM"),
		},
	}, nil
}

func FromEnvironment() (Config, error) {
	fileValues, err := loadDotEnv(".env")
	if err != nil {
		return Config{}, err
	}
	getenv := func(key string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		return fileValues[key]
	}
	return Load(getenv)
}

func loadDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			unquoted, err := strconv.Unquote(value)
			if err == nil {
				value = unquoted
			} else {
				value = value[1 : len(value)-1]
			}
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
