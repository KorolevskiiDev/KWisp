package bootstrap

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config is the logstore configuration.
type Config struct {
	Server         ServerConfig `yaml:"server"`
	Storage        StorageConfig `yaml:"storage"`
	AdminToken     string        `yaml:"admin_token"`
	PublicEndpoint string        `yaml:"public_endpoint"`
}

type ServerConfig struct {
	Address string `yaml:"address"`
}

type StorageConfig struct {
	Dir      string `yaml:"dir"`
	Capacity int    `yaml:"capacity"`
}

// Load reads and parses a YAML config file, falling back to environment variables.
func Load(path string) (*Config, error) {
	// First, try to load from YAML file if specified
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			var cfg Config
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("parse config: %w", err)
			}
			cfg.applyEnvOverrides()
			cfg.setDefaults()
			return &cfg, nil
		}
	}

	// Fall back to environment variables
	cfg := loadFromEnv()
	cfg.setDefaults()
	return cfg, nil
}

// loadFromEnv creates config from environment variables.
func loadFromEnv() *Config {
	cfg := &Config{
		Server: ServerConfig{
			Address: envString("LOGSTORE_SERVER_ADDRESS", ":8090"),
		},
		Storage: StorageConfig{
			Dir:      envString("LOGSTORE_STORAGE_DIR", "./data/logstore"),
			Capacity: envInt("LOGSTORE_STORAGE_CAPACITY", 5000),
		},
		AdminToken:     envString("LOGSTORE_ADMIN_TOKEN", ""),
		PublicEndpoint: envString("LOGSTORE_PUBLIC_ENDPOINT", ""),
	}
	return cfg
}

// applyEnvOverrides updates config with environment variables (for mixed YAML+env mode).
func (c *Config) applyEnvOverrides() {
	if v := envString("LOGSTORE_SERVER_ADDRESS", ""); v != "" {
		c.Server.Address = v
	}
	if v := envString("LOGSTORE_STORAGE_DIR", ""); v != "" {
		c.Storage.Dir = v
	}
	if v := envInt("LOGSTORE_STORAGE_CAPACITY", 0); v > 0 {
		c.Storage.Capacity = v
	}
	if v := envString("LOGSTORE_ADMIN_TOKEN", ""); v != "" {
		c.AdminToken = v
	}
	if v := envString("LOGSTORE_PUBLIC_ENDPOINT", ""); v != "" {
		c.PublicEndpoint = v
	}
}

func (c *Config) setDefaults() {
	if c.Server.Address == "" {
		c.Server.Address = ":8090"
	}
	if c.Storage.Dir == "" {
		c.Storage.Dir = "./data/logstore"
	}
	if c.Storage.Capacity == 0 {
		c.Storage.Capacity = 5000
	}
}

// envString returns value of env var or default.
func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envInt returns value of env var as int or default.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
