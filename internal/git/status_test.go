package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newRepo builds a real git repository in temp, so we can test everything
// actually works with a real Git command.
func newRepo(t *testing.T) *Repo {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test User")
	// We need to make sure GPG isn't enabled during test as thgere's now way
	// for the pinentry prompt to work in a non-interactive test env.
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	return New(dir)
}

// mustGit runs a git command against `dir` which must succeed, or it fails the test.
//
// The `git` package has its own copy of this, but test helpers are not exported across packages,
// so the ui tests need their own for setting up real repositories.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// We test committing an actual file, so we can make changes to it later in the test suite.
func commitFile(t *testing.T, repo *Repo, rel, content string) {
	t.Helper()
	writeFile(t, repo.Path, rel, content)
	mustGit(t, repo.Path, "add", "--", rel)
	mustGit(t, repo.Path, "commit", "-m", "add "+rel)
}

func statusOf(t *testing.T, repo *Repo) []FileStatus {
	t.Helper()
	got, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func findStatus(t *testing.T, entries []FileStatus, path string) FileStatus {
	t.Helper()
	for _, entry := range entries {
		if entry.Path == path {
			return entry
		}
	}
	t.Fatalf("no status for %q; got %+v", path, entries)
	return FileStatus{}
}

func TestStatusOnCleanRepoIsEmpty(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")

	if got := statusOf(t, repo); len(got) != 0 {
		t.Errorf("Status = %+v, want empty", got)
	}
}

func TestStatusReportsModifiedTrackedFileAsChanged(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, ".zshrc", "export A=2\n")

	got := findStatus(t, statusOf(t, repo), ".zshrc")

	if got.Untracked {
		t.Error("Untracked = true, want false")
	}
	if !got.Unstaged() {
		t.Error("Unstaged() = false, want true")
	}
	if got.Staged() {
		t.Error("Staged() = true, want false")
	}
}

func TestStatusReportsUntrackedFileAsNew(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, ".newfile", "secret\n")

	got := findStatus(t, statusOf(t, repo), ".newfile")

	if !got.Untracked {
		t.Error("Untracked = false, want true")
	}
}

// Git --porcelain quotes paths containing spaces unless `-z` is specified, so we need to make sure
// -z is always present.
func TestStatusParsesPathWithSpaces(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, "my notes file.txt", "hello\n")

	got := findStatus(t, statusOf(t, repo), "my notes file.txt")

	if !got.Untracked {
		t.Error("Untracked = false, want true")
	}
}

func TestStatusParsesPathWithQuoteCharacter(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, `od"d.txt`, "hello\n")

	findStatus(t, statusOf(t, repo), `od"d.txt`)
}

func TestStatusReportsStagedNewFile(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, ".added", "x\n")
	mustGit(t, repo.Path, "add", "--", ".added")

	got := findStatus(t, statusOf(t, repo), ".added")

	if !got.Staged() {
		t.Error("Staged() = false, want true")
	}
	if got.Untracked {
		t.Error("Untracked = true, want false")
	}
}

func TestStatusReportsFileStagedThenModifiedAgain(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, ".zshrc", "export A=2\n")
	mustGit(t, repo.Path, "add", "--", ".zshrc")
	writeFile(t, repo.Path, ".zshrc", "export A=3\n")

	got := findStatus(t, statusOf(t, repo), ".zshrc")

	if !got.Staged() {
		t.Error("Staged() = false, want true")
	}
	if !got.Unstaged() {
		t.Error("Unstaged() = false, want true")
	}
}

// A rename emits two NUL-separated paths when `-z` is specified. Mishandling it shifts
// every following entry by one field, so we need to make sure we're handling that correctly.
func TestStatusParsesRenameAndKeepsFollowingEntries(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, "old-name.txt", "content\n")
	commitFile(t, repo, "zz-later.txt", "other\n")
	mustGit(t, repo.Path, "mv", "old-name.txt", "new-name.txt")
	writeFile(t, repo.Path, "zz-later.txt", "changed\n")

	entries := statusOf(t, repo)

	renamed := findStatus(t, entries, "new-name.txt")
	if renamed.OldPath != "old-name.txt" {
		t.Errorf("OldPath = %q, want %q", renamed.OldPath, "old-name.txt")
	}
	// The entry after the rename must still be parsed as its own record.
	later := findStatus(t, entries, "zz-later.txt")
	if !later.Unstaged() {
		t.Error("zz-later.txt Unstaged() = false, want true")
	}
}

// The `-uall` option makes sure new files within a new directory are included in the status output,
// by default Git would only show the new directory, and ignore the files, so we wouldn't be able
// to show them to the user.
func TestStatusListsFilesInsideUntrackedDirectory(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, "newdir/a.txt", "a\n")
	writeFile(t, repo.Path, "newdir/b.txt", "b\n")

	entries := statusOf(t, repo)

	findStatus(t, entries, "newdir/a.txt")
	findStatus(t, entries, "newdir/b.txt")
	for _, entry := range entries {
		if entry.Path == "newdir/" || entry.Path == "newdir" {
			t.Errorf("got a directory entry %q, want individual files", entry.Path)
		}
	}
}

func TestStatusOmitsIgnoredFiles(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".gitignore", "ignored.txt\n")
	writeFile(t, repo.Path, "ignored.txt", "junk\n")

	for _, entry := range statusOf(t, repo) {
		if entry.Path == "ignored.txt" {
			t.Errorf("ignored.txt appears in status: %+v", entry)
		}
	}
}

func TestStatusReportsDeletedFile(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	if err := os.Remove(filepath.Join(repo.Path, ".zshrc")); err != nil {
		t.Fatal(err)
	}

	got := findStatus(t, statusOf(t, repo), ".zshrc")

	if !got.Deleted() {
		t.Error("Deleted() = false, want true")
	}
}
