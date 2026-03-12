// Package config handles loading and parsing the configuration file.
package config

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Top-level keys
type Config struct {
	Server             ServerConfig                       `yaml:"server"`
	Providers          map[string]ProviderConfig          `yaml:"providers"`
	RateLimit          RateLimitConfig                    `yaml:"rate_limit"`
	ProviderRateLimits map[string]ProviderRateLimitConfig `yaml:"provider_rate_limits"`
	CircuitBreaker     CircuitBreakerConfig               `yaml:"circuit_breaker"`
	Redis              RedisConfig                        `yaml:"redis"`
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
	TrustedProxies  []string      `yaml:"trusted_proxies"`
}

// ProviderRateLimitConfig holds per-provider aggregate rate limit settings.
type ProviderRateLimitConfig struct {
	RPM   float64 `yaml:"rpm"`   // Requests per minute (matches provider quota docs).
	Burst int     `yaml:"burst"` // Max burst size.
}

// CircuitBreakerConfig holds per-provider circuit breaker settings.
type CircuitBreakerConfig struct {
	MaxRequests uint32        `yaml:"max_requests"`
	Interval    time.Duration `yaml:"interval"`
	Timeout     time.Duration `yaml:"timeout"`
}

// RedisConfig holds Redis connection settings.
// When Addr is empty, the gateway falls back to in-memory state.
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// Matches ${ENV_VAR} placeholders.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads a YAML config file from path, expands any ${ENV_VAR}
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %s: %w", path, err)
	}

	// Replace env variables with their values (expand placeholders).
	expanded := expandEnvVars(string(data))

	// Pass the expanded YAML to yaml (Unmarshal).
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml: %w", err)
	}

	return &cfg, nil
}

// Validate checks that the loaded configuration has all required fields
// and sane value ranges. Call immediately after Load().
func (c *Config) Validate() error {
	// Server address.
	if c.Server.Address == "" {
		return fmt.Errorf("config: server.address is required")
	}

	// At least one provider must be configured.
	if len(c.Providers) == 0 {
		return fmt.Errorf("config: at least one provider must be configured")
	}

	// Each provider needs non-empty credentials.
	for name, p := range c.Providers {
		if p.APIKey == "" {
			return fmt.Errorf("config: provider %q has empty api_key (check env vars)", name)
		}
		if p.BaseURL == "" {
			return fmt.Errorf("config: provider %q has empty base_url", name)
		}
	}

	// Rate limit values must be non-negative.
	if c.RateLimit.RPS < 0 {
		return fmt.Errorf("config: rate_limit.rps must be >= 0, got %.2f", c.RateLimit.RPS)
	}
	if c.RateLimit.Burst < 0 {
		return fmt.Errorf("config: rate_limit.burst must be >= 0, got %d", c.RateLimit.Burst)
	}

	// Provider rate limits must reference known providers.
	for name, rl := range c.ProviderRateLimits {
		if _, ok := c.Providers[name]; !ok {
			return fmt.Errorf("config: provider_rate_limits references unknown provider %q", name)
		}
		if rl.RPM < 0 {
			return fmt.Errorf("config: provider_rate_limits.%s.rpm must be >= 0", name)
		}
	}

	return nil
}

// expandEnvVars replaces every ${VAR} in s with the corresponding
// environment variable value. Missing variables resolve to "".
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		val, ok := os.LookupEnv(varName)
		if !ok {
			log.Printf("WARNING: environment variable %s is not set", varName)
		}
		return val
	})
}
