package version

import "testing"

func TestDefaults_AreSet(t *testing.T) {
	if Version == "" {
		t.Fatal("Version should not be empty")
	}
	if Commit == "" {
		t.Fatal("Commit should not be empty")
	}
	if BuildDate == "" {
		t.Fatal("BuildDate should not be empty")
	}
}

func TestVariables_AreOverridableAtRuntime(t *testing.T) {
	origVersion, origCommit, origBuildDate := Version, Commit, BuildDate
	defer func() {
		Version = origVersion
		Commit = origCommit
		BuildDate = origBuildDate
	}()

	Version = "v1.2.3"
	Commit = "abc1234"
	BuildDate = "2026-04-01T00:00:00Z"

	if Version != "v1.2.3" {
		t.Fatalf("Version: got %q", Version)
	}
	if Commit != "abc1234" {
		t.Fatalf("Commit: got %q", Commit)
	}
	if BuildDate != "2026-04-01T00:00:00Z" {
		t.Fatalf("BuildDate: got %q", BuildDate)
	}
}

