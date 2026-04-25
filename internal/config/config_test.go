package config

import "testing"

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("AGENTX_ADDR", "")
	t.Setenv("AGENTX_DATA_DIR", "")
	t.Setenv("AGENTX_SQLITE_PATH", "")
	t.Setenv("AGENTX_ADMIN_TOKEN", "")

	cfg := FromEnv()

	if cfg.Addr != "127.0.0.1:8080" {
		t.Fatalf("Addr = %q, want 127.0.0.1:8080", cfg.Addr)
	}
	if cfg.DataDir != ".agentx" {
		t.Fatalf("DataDir = %q, want .agentx", cfg.DataDir)
	}
	if cfg.SQLitePath != ".agentx/agentx.db" {
		t.Fatalf("SQLitePath = %q, want .agentx/agentx.db", cfg.SQLitePath)
	}
	if cfg.AdminToken == "" {
		t.Fatal("AdminToken should have a generated token when unset")
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("AGENTX_ADDR", "0.0.0.0:9000")
	t.Setenv("AGENTX_DATA_DIR", "/tmp/agentx")
	t.Setenv("AGENTX_SQLITE_PATH", "/tmp/agentx/custom.db")
	t.Setenv("AGENTX_ADMIN_TOKEN", "dev-token")

	cfg := FromEnv()

	if cfg.Addr != "0.0.0.0:9000" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
	if cfg.DataDir != "/tmp/agentx" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.SQLitePath != "/tmp/agentx/custom.db" {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
	if cfg.AdminToken != "dev-token" {
		t.Fatalf("AdminToken = %q", cfg.AdminToken)
	}
}
