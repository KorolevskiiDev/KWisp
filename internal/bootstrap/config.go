package bootstrap

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the logstore configuration.
type Config struct {
	Server         ServerConfig  `yaml:"server"`
	Storage        StorageConfig `yaml:"storage"`
	AdminToken     string        `yaml:"admin_token"`
	PublicEndpoint string        `yaml:"public_endpoint"`
	CORS           CORSConfig    `yaml:"cors"`
}

type ServerConfig struct {
	Address string `yaml:"address"`
}

type StorageConfig struct {
	Dir      string `yaml:"dir"`
	Capacity int    `yaml:"capacity"`
}

type CORSConfig struct {
	Origins []string `yaml:"origins"`
}

// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.setDefaults()
	return &cfg, nil
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
	if len(c.CORS.Origins) == 0 {
		c.CORS.Origins = []string{"*"}
	}
}
