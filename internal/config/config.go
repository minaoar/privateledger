package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the application configuration
type Config struct {
	Version      int           `json:"version"`
	Server       ServerConfig  `json:"server"`
	Logging      LoggingConfig `json:"logging"`
	StartOfMonth int           `json:"start_of_month"`
	DebugMode    bool          `json:"debug_mode"`
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Port            int  `json:"port"`
	AutoOpenBrowser bool `json:"auto_open_browser"`
}

// LoggingConfig holds logging-specific configuration
type LoggingConfig struct {
	EnableFileLogging bool   `json:"enable_file_logging"`
	LogFilePath       string `json:"log_file_path"`
	LogLevel          string `json:"log_level"` // "debug", "info", "warn", "error"
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		Version: 1,
		Server: ServerConfig{
			Port:            8844,
			AutoOpenBrowser: true,
		},
		Logging: LoggingConfig{
			EnableFileLogging: false,
			LogFilePath:       "server.log",
			LogLevel:          "debug",
		},
		StartOfMonth: 1,
		DebugMode:    false,
	}
}

// Load reads the config file or creates a default one if it doesn't exist
func Load(configPath string) (*Config, error) {
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		cfg := DefaultConfig()
		if err := Save(configPath, cfg); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		return cfg, nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate start_of_month
	if cfg.StartOfMonth < 1 || cfg.StartOfMonth > 28 {
		return nil, fmt.Errorf("start_of_month must be between 1 and 28, got %d", cfg.StartOfMonth)
	}

	return &cfg, nil
}

// Save writes the config to disk
func Save(configPath string, cfg *Config) error {
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
