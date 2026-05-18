package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_WithConfigFile(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configContent := `
server:
  port: 9090
  max_body_bytes: 2048
database:
  driver: sqlite
  dsn: test.db
cache:
  enabled: true
  ttl: 120
rate_limit:
  enabled: true
  requests_per_second: 10
  burst: 20
circuit_breaker:
  enabled: false
auth:
  enabled: true
  exempt_paths:
    - /metrics
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	// 设置环境变量指向配置文件
	os.Setenv("MCP_GATEWAY_CONFIG", configPath)
	defer os.Unsetenv("MCP_GATEWAY_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.MaxBodyBytes != 2048 {
		t.Errorf("expected max_body_bytes 2048, got %d", cfg.Server.MaxBodyBytes)
	}
	if cfg.RateLimit.RequestsPerSecond != 10 {
		t.Errorf("expected rps 10, got %f", cfg.RateLimit.RequestsPerSecond)
	}
	if cfg.Auth.Enabled != true {
		t.Errorf("expected auth enabled, got false")
	}
	if len(cfg.Auth.ExemptPaths) != 1 || cfg.Auth.ExemptPaths[0] != "/metrics" {
		t.Errorf("expected exempt_paths [/metrics], got %v", cfg.Auth.ExemptPaths)
	}
}

func TestLoad_DefaultMaxBodyBytes(t *testing.T) {
	tmpDir := t.TempDir()
	configContent := `
server:
  port: 8080
database:
  driver: sqlite
  dsn: test.db
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	os.Setenv("MCP_GATEWAY_CONFIG", configPath)
	defer os.Unsetenv("MCP_GATEWAY_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Server.MaxBodyBytes != 1<<20 {
		t.Errorf("expected default MaxBodyBytes 1MB (1048576), got %d", cfg.Server.MaxBodyBytes)
	}
}
