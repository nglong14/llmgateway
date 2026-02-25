package provider

import (
	"fmt"
	"strings"
	"sync"
)

//Maps model names/prefixes to providers
type Registry struct {
	mu sync.RWMutex //Allows concurrent reads
	providers map[string]Provider //name -> provider
	prefixes []prefixEntry //ordered prefix rules
}

type prefixEntry struct {
	prefix string
	provider Provider
}

//NewRegistry creates an empty registry
func NewRegistry() *Registry{
	return &Registry{
		providers: make(map[string]Provider),
	}
}

//Adds a provider and its model prefixes to the registry
func (r * Registry) Register(p Provider, prefixes ...string){
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[p.Name()] = p
    for _, prefix := range prefixes {
        r.prefixes = append(r.prefixes, prefixEntry{prefix: prefix, provider: p})
    }
}

//Finds the provider with the following priority: explicit provider -> prefix -> error
func (r *Registry) Resolve(model, explicitProvider string) (Provider, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // 1. Explicit provider override.
    if explicitProvider != "" {
        if p, ok := r.providers[explicitProvider]; ok {
            return p, nil
        }
        return nil, fmt.Errorf("unknown provider: %q", explicitProvider)
    }

    // 2. Prefix matching.
    for _, entry := range r.prefixes {
        if strings.HasPrefix(model, entry.prefix) {
            return entry.provider, nil
        }
    }

    return nil, fmt.Errorf("no provider found for model: %q", model)
}

// ListAll returns all registered providers.
func (r *Registry) ListAll() []Provider {
    r.mu.RLock()
    defer r.mu.RUnlock()
    result := make([]Provider, 0, len(r.providers))
    for _, p := range r.providers {
        result = append(result, p)
    }
    return result
}