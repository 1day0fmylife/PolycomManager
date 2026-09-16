package config

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	Listen   string
	DataDir  string
	OpenUI   bool
	LogLevel string
}

func Load() Config {
	defaultData := defaultDataDir()
	cfg := Config{}
	flag.StringVar(&cfg.Listen, "listen", "127.0.0.1:8080", "HTTP listen address")
	flag.StringVar(&cfg.DataDir, "data-dir", defaultData, "data directory")
	flag.BoolVar(&cfg.OpenUI, "open-ui", true, "open browser after startup")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "log level: debug|info|warn|error")
	flag.Parse()
	return cfg
}

func defaultDataDir() string {
	if v := os.Getenv("POLYCOM_MANAGER_DATA_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("PROGRAMDATA"); v != "" {
			return filepath.Join(v, "PolycomManager")
		}
	}
	if runtime.GOOS == "linux" {
		if os.Geteuid() == 0 {
			return "/var/lib/polycom-manager"
		}
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".local", "share", "polycom-manager")
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".polycom-manager")
	}
	return "./data"
}
