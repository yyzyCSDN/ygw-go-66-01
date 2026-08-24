package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

// newFlagSet 创建独立的命令行解析器，避免多次加载配置时与全局 flag 冲突。
func newFlagSet(cfg *Config) *flag.FlagSet {
	fs := flag.NewFlagSet("auditlog", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	fs.StringVar(&cfg.DataDir, "dir", cfg.DataDir, "data directory")
	fs.IntVar(&cfg.BlockSize, "block-size", cfg.BlockSize, "records per block")
	fs.StringVar(&cfg.Retention, "retention", cfg.Retention, "retention policy (e.g. 30d, 7d, 24h)")
	fs.DurationVar(&cfg.RetentionInterval, "retention-interval", cfg.RetentionInterval, "retention scan interval")
	fs.IntVar(&cfg.ExportLimit, "export-limit", cfg.ExportLimit, "records per export batch")
	fs.StringVar(&cfg.WebRoot, "web-root", cfg.WebRoot, "directory containing console.html")
	return fs
}

// Config 汇总命令行与环境变量配置。
type Config struct {
	Addr              string
	DataDir           string
	BlockSize         int
	Retention         string
	RetentionInterval time.Duration
	ExportLimit       int
	WebRoot           string
}

// LoadConfig 从命令行参数与环境变量读取配置。
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Addr:              "127.0.0.1:8090",
		DataDir:           "data",
		BlockSize:         256,
		Retention:         "30d",
		RetentionInterval: time.Hour,
		ExportLimit:       100,
		WebRoot:           "web",
	}
	fs := newFlagSet(cfg)
	if err := fs.Parse(os.Args[1:]); err != nil {
		if flag.Lookup("test.v") == nil {
			return nil, err
		}
	}

	if value := os.Getenv("AUDITLOG_ADDR"); value != "" {
		cfg.Addr = value
	}
	if value := os.Getenv("AUDITLOG_DATA_DIR"); value != "" {
		cfg.DataDir = value
	}
	if value := os.Getenv("AUDITLOG_BLOCK_SIZE"); value != "" {
		number, err := strconv.Atoi(value)
		if err != nil || number <= 0 {
			return nil, fmt.Errorf("invalid AUDITLOG_BLOCK_SIZE %q", value)
		}
		cfg.BlockSize = number
	}
	if value := os.Getenv("AUDITLOG_RETENTION"); value != "" {
		cfg.Retention = value
	}
	if cfg.Addr == "" || cfg.DataDir == "" {
		return nil, fmt.Errorf("addr and data directory must not be empty")
	}
	if cfg.BlockSize <= 0 || cfg.ExportLimit <= 0 {
		return nil, fmt.Errorf("block size and export limit must be positive")
	}
	return cfg, nil
}
