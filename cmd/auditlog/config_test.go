package main

import (
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Addr == "" || cfg.DataDir == "" || cfg.BlockSize <= 0 {
		t.Fatalf("invalid default config: %+v", cfg)
	}
}

func TestLoadConfigEnvOverride(t *testing.T) {
	t.Setenv("AUDITLOG_ADDR", "127.0.0.1:9999")
	t.Setenv("AUDITLOG_BLOCK_SIZE", "64")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9999" || cfg.BlockSize != 64 {
		t.Fatalf("env override failed: %+v", cfg)
	}
}

func TestLoadConfigRejectsInvalidBlockSizeEnv(t *testing.T) {
	t.Setenv("AUDITLOG_BLOCK_SIZE", "not-a-number")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected error for invalid block size")
	}
}

func TestConfigDurations(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.RetentionInterval <= 0 || cfg.ExportLimit <= 0 {
		t.Fatalf("invalid durations: %+v", cfg)
	}
}
