package pkg

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds application configuration that must exist before connecting to the database.
type Config struct {
	DatabaseURL string `json:"database_url"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".twist")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

// LoadConfig reads the config from ~/.twist/config.json.
// Returns an empty Config (not an error) if the file does not exist.
func LoadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &Config{}, nil
	}
	return &cfg, nil
}

// SaveConfig writes the config to ~/.twist/config.json.
func SaveConfig(cfg *Config) error {
	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0644)
}
