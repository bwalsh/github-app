// Package tenant manages the multi-tenant registry mapping
// (installation_id, repository_id) → Tenant.
package tenant

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

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

// Persistence defines the pluggable storage contract for tenant mappings.
type Persistence interface {
	Register(key Key, t *Tenant) error
	Lookup(key Key) (*Tenant, bool, error)
	Unregister(key Key) error
	Close() error
}

// Registry is a thin wrapper around a pluggable tenant persistence backend.
type Registry struct {
	p Persistence
}

// New returns a registry using in-memory persistence.
func New() *Registry {
	return NewWithPersistence(newMemoryPersistence())
}

// NewWithPersistence returns a registry backed by a caller-provided persistence backend.
func NewWithPersistence(p Persistence) *Registry {
	return &Registry{p: p}
}

// NewSQLitePersistence creates a SQLite-backed tenant persistence implementation.
func NewSQLitePersistence(dsn string) (Persistence, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	s := &sqlitePersistence{db: db}
	if err := s.exec(`
		CREATE TABLE IF NOT EXISTS tenants (
			installation_id INTEGER NOT NULL,
			repository_id   INTEGER NOT NULL,
			name            TEXT NOT NULL,
			namespace       TEXT NOT NULL,
			PRIMARY KEY (installation_id, repository_id)
		)
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create tenants table: %w", err)
	}

	return s, nil
}

// Register adds or replaces the tenant mapping for key.
func (r *Registry) Register(key Key, t *Tenant) error {
	if r == nil || r.p == nil {
		return errors.New("tenant registry persistence is not configured")
	}
	return r.p.Register(key, t)
}

// Lookup returns the Tenant for key, or (nil, false, nil) if not found.
func (r *Registry) Lookup(key Key) (*Tenant, bool, error) {
	if r == nil || r.p == nil {
		return nil, false, errors.New("tenant registry persistence is not configured")
	}
	return r.p.Lookup(key)
}

// Unregister removes the tenant mapping for key.
func (r *Registry) Unregister(key Key) error {
	if r == nil || r.p == nil {
		return errors.New("tenant registry persistence is not configured")
	}
	return r.p.Unregister(key)
}

// Close releases resources held by the configured persistence backend.
func (r *Registry) Close() error {
	if r == nil || r.p == nil {
		return nil
	}
	return r.p.Close()
}

type memoryPersistence struct {
	mu      sync.RWMutex
	tenants map[Key]*Tenant
}

func newMemoryPersistence() *memoryPersistence {
	return &memoryPersistence{tenants: make(map[Key]*Tenant)}
}

func (m *memoryPersistence) Register(key Key, t *Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tenants[key] = t
	return nil
}

func (m *memoryPersistence) Lookup(key Key) (*Tenant, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenants[key]
	return t, ok, nil
}

func (m *memoryPersistence) Unregister(key Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tenants, key)
	return nil
}

func (m *memoryPersistence) Close() error {
	return nil
}

type sqlitePersistence struct {
	mu sync.Mutex
	db *sql.DB
}

func (s *sqlitePersistence) Register(key Key, t *Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.exec(
		`INSERT INTO tenants (installation_id, repository_id, name, namespace)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(installation_id, repository_id) DO UPDATE SET
		   name=excluded.name,
		   namespace=excluded.namespace`,
		key.InstallationID, key.RepositoryID, t.Name, t.Namespace,
	)
	if err != nil {
		return fmt.Errorf("upsert tenant: %w", err)
	}
	return nil
}

func (s *sqlitePersistence) Lookup(key Key) (*Tenant, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(
		`SELECT name, namespace FROM tenants WHERE installation_id = ? AND repository_id = ? LIMIT 1`,
		key.InstallationID, key.RepositoryID,
	)

	var out Tenant
	if err := row.Scan(&out.Name, &out.Namespace); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("lookup tenant: %w", err)
	}

	return &out, true, nil
}

func (s *sqlitePersistence) Unregister(key Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.exec(
		`DELETE FROM tenants WHERE installation_id = ? AND repository_id = ?`,
		key.InstallationID, key.RepositoryID,
	)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	return nil
}

func (s *sqlitePersistence) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *sqlitePersistence) exec(sqlStmt string, args ...any) error {
	_, err := s.db.Exec(sqlStmt, args...)
	if err != nil {
		return err
	}
	return nil
}

