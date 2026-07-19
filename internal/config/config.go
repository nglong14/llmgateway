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

type Config struct {
	Server             ServerConfig                       `yaml:"server"`
	Providers          map[string]ProviderConfig          `yaml:"providers"`
	RateLimit          RateLimitConfig                    `yaml:"rate_limit"`
	ProviderRateLimits map[string]ProviderRateLimitConfig `yaml:"provider_rate_limits"`
	CircuitBreaker     CircuitBreakerConfig               `yaml:"circuit_breaker"`
	Redis              RedisConfig                        `yaml:"redis"`
	Auth               AuthConfig                         `yaml:"auth"`
	Database           DatabaseConfig                     `yaml:"database"`
	JWT                JWTConfig                          `yaml:"jwt"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

func (c DatabaseConfig) Configured() bool {
	return c.Host != "" && c.User != "" && c.DBName != ""
}

type JWTConfig struct {
	AccessTokenSecret  string        `yaml:"access_token_secret"`
	RefreshTokenSecret string        `yaml:"refresh_token_secret"`
	AccessTokenTTL     time.Duration `yaml:"access_token_ttl"`
	RefreshTokenTTL    time.Duration `yaml:"refresh_token_ttl"`
}

type AuthConfig struct {
	Enabled   bool           `yaml:"enabled"`
	Keys      []APIKeyConfig `yaml:"keys"`
	SkipPaths []string       `yaml:"skip_paths"`
}

type APIKeyConfig struct {
	KeyHash string `yaml:"key_hash"`
	Name    string `yaml:"name"`
}

type ServerConfig struct {
	Address string `yaml:"address"`
}

type ProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

type RateLimitConfig struct {
	RPS             float64       `yaml:"rps"`
	Burst           int           `yaml:"burst"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
	TrustedProxies  []string      `yaml:"trusted_proxies"`
}

type ProviderRateLimitConfig struct {
	RPM   float64 `yaml:"rpm"`
	Burst int     `yaml:"burst"`
}

type CircuitBreakerConfig struct {
	MaxRequests uint32        `yaml:"max_requests"`
	Interval    time.Duration `yaml:"interval"`
	Timeout     time.Duration `yaml:"timeout"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %s: %w", path, err)
	}

	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Auth.Enabled {
		if len(c.Auth.Keys) == 0 {
			return fmt.Errorf("config: auth.enabled is true but no keys are configured")
		}
		for i, k := range c.Auth.Keys {
			if k.KeyHash == "" {
				return fmt.Errorf("config: auth.keys[%d] has empty key_hash (check env vars)", i)
			}
			if !isValidSHA256Hex(k.KeyHash) {
				return fmt.Errorf("config: auth.keys[%d] key_hash is not a valid SHA-256 hex digest", i)
			}
		}
	}
	if c.Server.Address == "" {
		return fmt.Errorf("config: server.address is required")
	}

	if len(c.Providers) == 0 {
		return fmt.Errorf("config: at least one provider must be configured")
	}

	for name, p := range c.Providers {
		if p.APIKey == "" {
			return fmt.Errorf("config: provider %q has empty api_key (check env vars)", name)
		}
		if p.BaseURL == "" {
			return fmt.Errorf("config: provider %q has empty base_url", name)
		}
	}

	if c.RateLimit.RPS < 0 {
		return fmt.Errorf("config: rate_limit.rps must be >= 0, got %.2f", c.RateLimit.RPS)
	}
	if c.RateLimit.Burst < 0 {
		return fmt.Errorf("config: rate_limit.burst must be >= 0, got %d", c.RateLimit.Burst)
	}

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

func isValidSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

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
