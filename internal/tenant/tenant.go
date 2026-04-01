// Package tenant manages the multi-tenant registry mapping
// (installation_id, repository_id) → Tenant.
package tenant

import "sync"

// Key identifies a tenant by GitHub installation ID and repository ID.
type Key struct {
	InstallationID int64
	RepositoryID   int64
}

// Tenant represents an isolated tenant scoped to a GitHub installation and repository.
type Tenant struct {
	Name      string // human-readable name, e.g. "tenant-123-456"
	Namespace string // execution namespace, e.g. "ns-456"
}

// Registry is a thread-safe in-memory store mapping Key → Tenant.
type Registry struct {
	mu      sync.RWMutex
	tenants map[Key]*Tenant
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{tenants: make(map[Key]*Tenant)}
}

// Register adds or replaces the tenant mapping for key.
func (r *Registry) Register(key Key, t *Tenant) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tenants[key] = t
}

// Lookup returns the Tenant for key, or (nil, false) if not found.
func (r *Registry) Lookup(key Key) (*Tenant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tenants[key]
	return t, ok
}

// Unregister removes the tenant mapping for key.
func (r *Registry) Unregister(key Key) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tenants, key)
}

