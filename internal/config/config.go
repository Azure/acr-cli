// Package config provides configuration management for acr-cli.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the configuration settings for acr-cli.
type Config struct {
	// AbacBatchSize is the number of repositories to batch together when requesting tokens for ABAC registries.
	// Default: 10
	AbacBatchSize int `yaml:"abacBatchSize"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		AbacBatchSize: 10,
	}
}

// configPath is the path to the config file at the project root.
const configPath = "config.yaml"

// Load reads the configuration from the config file.
// If no config file is found, it returns the default configuration.
func Load() *Config {
	cfg := DefaultConfig()

	if data, err := os.ReadFile(configPath); err == nil {
		// Found a config file, parse it
		if err := yaml.Unmarshal(data, cfg); err == nil {
			return cfg
		}
	}

	return cfg
}
