package app

import (
	"github.com/nglong14/llmgateway/internal/middleware"
	"github.com/nglong14/llmgateway/internal/provider"
	"github.com/nglong14/llmgateway/internal/provider/anthropic"
	"github.com/nglong14/llmgateway/internal/provider/deepseek"
	"github.com/nglong14/llmgateway/internal/provider/gemini"
	"github.com/nglong14/llmgateway/internal/provider/openai"
)

type providerSpec struct {
	name     string
	newFn    func(apiKey, baseURL string) provider.Provider
	prefixes []string
}


var providerSpecs = []providerSpec{
	{name: "openai", newFn: func(k, u string) provider.Provider { return openai.New(k, u) }, prefixes: []string{"gpt-", "o1-", "o3-", "o4-"}},
	{name: "gemini", newFn: func(k, u string) provider.Provider { return gemini.New(k, u) }, prefixes: []string{"gemini-", "g-"}},
	{name: "anthropic", newFn: func(k, u string) provider.Provider { return anthropic.New(k, u) }, prefixes: []string{"claude-"}},
	{name: "deepseek", newFn: func(k, u string) provider.Provider { return deepseek.New(k, u) }, prefixes: []string{"deepseek-"}},
}

func (s *Server) registerProviders() {
	s.registry = provider.NewRegistry()

	for _, spec := range providerSpecs {
		pc, ok := s.cfg.Providers[spec.name]
		if !ok {
			continue
		}
		client := spec.newFn(pc.APIKey, pc.BaseURL)
		s.registry.Register(s.wrapProvider(client, spec.name), spec.prefixes...)
		s.logger.Info("Registered provider: " + spec.name)
	}
}

// wrapProvider applies the decorator chain: circuit breaker, then an optional
// per-provider rate limiter (Redis-backed when available, in-memory otherwise).
func (s *Server) wrapProvider(p provider.Provider, name string) provider.Provider {
	wrapped := middleware.NewCircuitBreakerProvider(p, s.cfg.CircuitBreaker)

	rlCfg, ok := s.cfg.ProviderRateLimits[name]
	if !ok || rlCfg.RPM <= 0 {
		return wrapped
	}
	if s.redisClient != nil {
		return middleware.NewRedisRateLimitedProvider(wrapped, s.redisClient.RDB, rlCfg)
	}
	return middleware.NewRateLimitedProvider(wrapped, rlCfg)
}
