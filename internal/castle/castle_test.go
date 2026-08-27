package castle

import (
	"os"
	"path/filepath"
	"testing"
)

// mkCastle creates a castle skeleton: a .git directory and an empty home directory.
func mkCastle(t *testing.T, reposDir, name string) string {
	t.Helper()
	root := filepath.Join(reposDir, name)
	for _, dir := range []string{".git", "home"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDiscoverFindsSingleCastle(t *testing.T) {
	repos := t.TempDir()
	mkCastle(t, repos, "dotfiles")

	got, err := Discover(repos)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 castle, got %d: %+v", len(got), got)
	}
	if got[0].Name != "dotfiles" {
		t.Errorf("Name = %q, want %q", got[0].Name, "dotfiles")
	}
	if want := filepath.Join(repos, "dotfiles"); got[0].Root != want {
		t.Errorf("Root = %q, want %q", got[0].Root, want)
	}
	if want := filepath.Join(repos, "dotfiles", "home"); got[0].Home() != want {
		t.Errorf("Home() = %q, want %q", got[0].Home(), want)
	}
}

func TestDiscoverFindsSiblingCastles(t *testing.T) {
	repos := t.TempDir()
	mkCastle(t, repos, "dotfiles")
	mkCastle(t, repos, "work")

	got, err := Discover(repos)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 castles, got %d: %+v", len(got), got)
	}
	// Discover sorts by name, so the order is deterministic.
	if got[0].Name != "dotfiles" || got[1].Name != "work" {
		t.Errorf("names = %q, %q; want dotfiles, work", got[0].Name, got[1].Name)
	}
}

// A git submodule inside a castle has its own .git. It is not a castle.
func TestDiscoverRejectsNestedRepo(t *testing.T) {
	repos := t.TempDir()
	root := mkCastle(t, repos, "dotfiles")
	submodule := filepath.Join(root, "home", ".vim", "bundle", "fugitive", ".git")
	if err := os.MkdirAll(submodule, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(repos)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 castle, got %d: %+v", len(got), got)
	}
	if got[0].Name != "dotfiles" {
		t.Errorf("Name = %q, want %q", got[0].Name, "dotfiles")
	}
}

// A submodule records .git as a file containing a gitdir pointer, not a directory.
func TestDiscoverRejectsNestedRepoWithGitFile(t *testing.T) {
	repos := t.TempDir()
	root := mkCastle(t, repos, "dotfiles")
	bundle := filepath.Join(root, "home", ".vim", "bundle", "fugitive")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(bundle, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: ../../../../.git/modules/fugitive\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(repos)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 castle, got %d: %+v", len(got), got)
	}
}

func TestDiscoverIgnoresPlainDirectories(t *testing.T) {
	repos := t.TempDir()
	mkCastle(t, repos, "dotfiles")
	if err := os.MkdirAll(filepath.Join(repos, "notacastle", "home"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(repos)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 castle, got %d: %+v", len(got), got)
	}
}

func TestDiscoverOnMissingReposDirReturnsNoCastles(t *testing.T) {
	got, err := Discover(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("want no error for an absent repos dir, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 castles, got %d", len(got))
	}
}
