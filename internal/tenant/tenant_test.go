// Package tenant_test tests tenant persistence implementations.
package tenant_test

import (
	"path/filepath"
	"testing"

	"github.com/bwalsh/github-app/internal/tenant"
)

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := tenant.New()
	key := tenant.Key{InstallationID: 1, RepositoryID: 10}
	expected := &tenant.Tenant{Name: "tenant-1-10", Namespace: "ns-10"}

	if err := r.Register(key, expected); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	got, ok, err := r.Lookup(key)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if !ok {
		t.Fatal("expected tenant to be found")
	}
	if got.Name != expected.Name {
		t.Errorf("name: got %q, want %q", got.Name, expected.Name)
	}
	if got.Namespace != expected.Namespace {
		t.Errorf("namespace: got %q, want %q", got.Namespace, expected.Namespace)
	}
}

func TestRegistry_LookupMissing(t *testing.T) {
	r := tenant.New()
	_, ok, err := r.Lookup(tenant.Key{InstallationID: 99, RepositoryID: 99})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if ok {
		t.Error("expected lookup to return false for unknown key")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := tenant.New()
	key := tenant.Key{InstallationID: 2, RepositoryID: 20}
	if err := r.Register(key, &tenant.Tenant{Name: "tenant-2-20", Namespace: "ns-20"}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := r.Unregister(key); err != nil {
		t.Fatalf("unregister failed: %v", err)
	}
	_, ok, err := r.Lookup(key)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if ok {
		t.Error("expected tenant to be removed after unregister")
	}
}

func TestRegistry_Isolation(t *testing.T) {
	r := tenant.New()
	keyA := tenant.Key{InstallationID: 1, RepositoryID: 10}
	keyB := tenant.Key{InstallationID: 1, RepositoryID: 20}

	if err := r.Register(keyA, &tenant.Tenant{Name: "a", Namespace: "ns-a"}); err != nil {
		t.Fatalf("register A failed: %v", err)
	}
	if err := r.Register(keyB, &tenant.Tenant{Name: "b", Namespace: "ns-b"}); err != nil {
		t.Fatalf("register B failed: %v", err)
	}

	a, _, err := r.Lookup(keyA)
	if err != nil {
		t.Fatalf("lookup A failed: %v", err)
	}
	b, _, err := r.Lookup(keyB)
	if err != nil {
		t.Fatalf("lookup B failed: %v", err)
	}

	if a.Name == b.Name {
		t.Error("tenants from different repos should be isolated")
	}
}

func TestSQLitePersistence_RegisterLookupAndUnregister(t *testing.T) {
	t.Parallel()

	dsn := filepath.Join(t.TempDir(), "tenants.db")
	p, err := tenant.NewSQLitePersistence(dsn)
	if err != nil {
		t.Fatalf("failed to create sqlite persistence: %v", err)
	}
	r := tenant.NewWithPersistence(p)
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
	})

	key := tenant.Key{InstallationID: 42, RepositoryID: 1001}
	tenantRecord := &tenant.Tenant{Name: "acme", Namespace: "ns-acme"}

	if err := r.Register(key, tenantRecord); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	got, ok, err := r.Lookup(key)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if !ok {
		t.Fatal("expected tenant to exist")
	}
	if got.Name != tenantRecord.Name || got.Namespace != tenantRecord.Namespace {
		t.Fatalf("unexpected tenant value: got=%+v want=%+v", got, tenantRecord)
	}

	if err := r.Unregister(key); err != nil {
		t.Fatalf("unregister failed: %v", err)
	}
	_, ok, err = r.Lookup(key)
	if err != nil {
		t.Fatalf("lookup after unregister failed: %v", err)
	}
	if ok {
		t.Fatal("expected tenant to be removed")
	}
}

func TestSQLitePersistence_PersistsAcrossRegistryInstances(t *testing.T) {
	t.Parallel()

	dsn := filepath.Join(t.TempDir(), "tenants.db")
	key := tenant.Key{InstallationID: 5, RepositoryID: 6}
	expected := &tenant.Tenant{Name: "persisted", Namespace: "ns-persisted"}

	p1, err := tenant.NewSQLitePersistence(dsn)
	if err != nil {
		t.Fatalf("create persistence #1 failed: %v", err)
	}
	r1 := tenant.NewWithPersistence(p1)
	if err := r1.Register(key, expected); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := r1.Close(); err != nil {
		t.Fatalf("close #1 failed: %v", err)
	}

	p2, err := tenant.NewSQLitePersistence(dsn)
	if err != nil {
		t.Fatalf("create persistence #2 failed: %v", err)
	}
	r2 := tenant.NewWithPersistence(p2)
	t.Cleanup(func() {
		if err := r2.Close(); err != nil {
			t.Fatalf("close #2 failed: %v", err)
		}
	})

	got, ok, err := r2.Lookup(key)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if !ok {
		t.Fatal("expected persisted tenant to exist")
	}
	if got.Name != expected.Name || got.Namespace != expected.Namespace {
		t.Fatalf("unexpected persisted tenant: got=%+v want=%+v", got, expected)
	}
}
