package config

import "testing"

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

	if _, err := Load(func(string) string { return "" }); err == nil {
		t.Fatal("missing DATABASE_URL accepted")
	}
}
