// Package registry handles the registration and resolution of providers.
package provider

import (
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	prefixes  []prefixEntry
}

type prefixEntry struct {
	prefix   string
	provider Provider
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func (r *Registry) Register(p Provider, prefixes ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[p.Name()] = p
	for _, prefix := range prefixes {
		r.prefixes = append(r.prefixes, prefixEntry{prefix: prefix, provider: p})
	}
}

func (r *Registry) Resolve(model, explicitProvider string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if explicitProvider != "" {
		if p, ok := r.providers[explicitProvider]; ok {
			return p, nil
		}
		return nil, fmt.Errorf("unknown provider: %q", explicitProvider)
	}

	for _, entry := range r.prefixes {
		if strings.HasPrefix(model, entry.prefix) {
			return entry.provider, nil
		}
	}

	return nil, fmt.Errorf("no provider found for model: %q", model)
}

func (r *Registry) ListAll() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}
