package castle

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeSubdirFile(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, SubdirFilename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSubdirsReadsLinesInOrder(t *testing.T) {
	repos := t.TempDir()
	root := mkCastle(t, repos, "dotfiles")
	writeSubdirFile(t, root, ".homesick/repos/dotfiles\n.exampleapp\n.config\n.config/exampletool\n")

	got, err := Subdirs(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".homesick/repos/dotfiles", ".exampleapp", ".config", ".config/exampletool"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Subdirs = %q, want %q", got, want)
	}
}

func TestSubdirsOnMissingFileReturnsEmpty(t *testing.T) {
	repos := t.TempDir()
	root := mkCastle(t, repos, "dotfiles")

	got, err := Subdirs(root)
	if err != nil {
		t.Fatalf("want no error for an absent subdir file, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Subdirs = %q, want empty", got)
	}
}

// A blank entry would make the linker treat the castle root as a subdir.
func TestSubdirsSkipsBlankLines(t *testing.T) {
	repos := t.TempDir()
	root := mkCastle(t, repos, "dotfiles")
	writeSubdirFile(t, root, ".exampleapp\n\n   \n.config\n")

	got, err := Subdirs(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".exampleapp", ".config"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Subdirs = %q, want %q", got, want)
	}
}

func TestSubdirsStripsCarriageReturns(t *testing.T) {
	repos := t.TempDir()
	root := mkCastle(t, repos, "dotfiles")
	writeSubdirFile(t, root, ".exampleapp\r\n.config\r\n")

	got, err := Subdirs(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".exampleapp", ".config"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Subdirs = %q, want %q", got, want)
	}
}

// Guard: a stray git init in the repos directory must not be treated as a real castle,
// as that would hide all the real ones.
func TestDiscoverIgnoresReposDirItself(t *testing.T) {
	repos := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repos, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
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
}
