// Package tenant_test tests the tenant registry.
package tenant_test

import (
	"testing"

	"github.com/bwalsh/github-app/internal/tenant"
)

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := tenant.New()
	key := tenant.Key{InstallationID: 1, RepositoryID: 10}
	expected := &tenant.Tenant{Name: "tenant-1-10", Namespace: "ns-10"}

	r.Register(key, expected)

	got, ok := r.Lookup(key)
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
	_, ok := r.Lookup(tenant.Key{InstallationID: 99, RepositoryID: 99})
	if ok {
		t.Error("expected lookup to return false for unknown key")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := tenant.New()
	key := tenant.Key{InstallationID: 2, RepositoryID: 20}
	r.Register(key, &tenant.Tenant{Name: "tenant-2-20", Namespace: "ns-20"})
	r.Unregister(key)
	_, ok := r.Lookup(key)
	if ok {
		t.Error("expected tenant to be removed after unregister")
	}
}

func TestRegistry_Isolation(t *testing.T) {
	r := tenant.New()
	keyA := tenant.Key{InstallationID: 1, RepositoryID: 10}
	keyB := tenant.Key{InstallationID: 1, RepositoryID: 20}

	r.Register(keyA, &tenant.Tenant{Name: "a", Namespace: "ns-a"})
	r.Register(keyB, &tenant.Tenant{Name: "b", Namespace: "ns-b"})

	a, _ := r.Lookup(keyA)
	b, _ := r.Lookup(keyB)

	if a.Name == b.Name {
		t.Error("tenants from different repos should be isolated")
	}
}

