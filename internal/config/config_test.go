package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndRequiresDatabase(t *testing.T) {
	cfg, err := Load(func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://localhost/agentdeck"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.Shutdown.String() != "10s" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if cfg.MigrationDatabaseURL != cfg.DatabaseURL {
		t.Fatalf("migration DSN should fall back to runtime DSN: %#v", cfg)
	}

	if _, err := Load(func(string) string { return "" }); err == nil {
		t.Fatal("missing DATABASE_URL accepted")
	}
}

func TestLoadDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# local config\nexport DATABASE_URL=postgres://localhost/app\nADDR=127.0.0.1:9090\nEMPTY=\"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	values, err := loadDotEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["DATABASE_URL"] != "postgres://localhost/app" || values["ADDR"] != "127.0.0.1:9090" {
		t.Fatalf("unexpected parsed values: %#v", values)
	}
	if values["EMPTY"] != "" {
		t.Fatalf("quoted empty value was not parsed: %#v", values)
	}
}

func TestLoadDotEnvMissingFileIsOptional(t *testing.T) {
	values, err := loadDotEnv(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if values != nil {
		t.Fatalf("missing file should return nil values: %#v", values)
	}
}
