package skillcapability

import (
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu           sync.RWMutex
	capabilities map[string]*Manifest
}

func NewRegistry() *Registry {
	return &Registry{capabilities: make(map[string]*Manifest)}
}

func (r *Registry) Register(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("skill capability manifest is nil")
	}
	if manifest.ID == "" {
		return fmt.Errorf("skill capability id is required")
	}
	if manifest.Version == "" {
		return fmt.Errorf("skill capability %s version is required", manifest.ID)
	}
	key := manifest.ID + "@" + manifest.Version

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.capabilities[key]; exists {
		return fmt.Errorf("skill capability %s already registered", key)
	}
	r.capabilities[key] = manifest
	return nil
}

func (r *Registry) List() []*Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]string, 0, len(r.capabilities))
	for key := range r.capabilities {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]*Manifest, 0, len(keys))
	for _, key := range keys {
		result = append(result, r.capabilities[key])
	}
	return result
}

func (r *Registry) MatchDomain(domain string) []*Manifest {
	all := r.List()
	if domain == "" {
		return all
	}
	matched := make([]*Manifest, 0, len(all))
	for _, cap := range all {
		if cap.Domain == domain {
			matched = append(matched, cap)
		}
	}
	return matched
}
