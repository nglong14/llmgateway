// Package config handles loading and parsing the configuration file.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Top-level keys
type Config struct {
	Server         ServerConfig              `yaml:"server"`
	Providers      map[string]ProviderConfig `yaml:"providers"`
	RateLimit      RateLimitConfig           `yaml:"rate_limit"`
	CircuitBreaker CircuitBreakerConfig      `yaml:"circuit_breaker"`
}

// HTTP server settings.
type ServerConfig struct {
	Address string `yaml:"address"`
}

// Config for each provider
type ProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

// RateLimitConfig holds per-IP token-bucket settings.
type RateLimitConfig struct {
	RPS             float64       `yaml:"rps"`
	Burst           int           `yaml:"burst"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
}

// CircuitBreakerConfig holds per-provider circuit breaker settings.
type CircuitBreakerConfig struct {
	MaxRequests uint32        `yaml:"max_requests"`
	Interval    time.Duration `yaml:"interval"`
	Timeout     time.Duration `yaml:"timeout"`
}

// Matches ${ENV_VAR} placeholders.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads a YAML config file from path, expands any ${ENV_VAR}
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %s: %w", path, err)
	}

	// Replace env variables with their values(expand placeholders).
	expanded := expandEnvVars(string(data))

	// Pass the expanded YAML to yaml (Unmarshal)
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml: %w", err)
	}

	return &cfg, nil
}

// expandEnvVars replaces every ${VAR} in s with the corresponding
// environment variable value. Missing variables resolve to "".
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		return os.Getenv(varName)
	})
}
