package git

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// addOrigin gives the repo a bare origin and pushes main to it.
func addOrigin(t *testing.T, repo *Repo) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(parent, "origin.git")

	if out, err := exec.Command("git", "init", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	mustGit(t, repo.Path, "remote", "add", "origin", bare)
	mustGit(t, repo.Path, "push", "-u", "origin", "main")
	return bare
}

// commitViaClone pushes a commit to bare from a separate clone, to simulate a
// remote change that needs to be pulled.
func commitViaClone(t *testing.T, bare, rel, content string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(dir, "clone")
	if out, err := exec.Command("git", "clone", bare, clone).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	mustGit(t, clone, "config", "user.email", "other@example.com")
	mustGit(t, clone, "config", "user.name", "Other User")
	mustGit(t, clone, "config", "commit.gpgsign", "false")
	writeFile(t, clone, rel, content)
	mustGit(t, clone, "add", "--", rel)
	mustGit(t, clone, "commit", "-m", "add "+rel)
	mustGit(t, clone, "push", "origin", "main")
}

func TestBranchReturnsCurrentBranch(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")

	got, err := repo.Branch()
	if err != nil {
		t.Fatal(err)
	}
	if got != "main" {
		t.Errorf("Branch = %q, want %q", got, "main")
	}
}

func TestRemoteURLReturnsOriginURL(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	bare := addOrigin(t, repo)

	got, err := repo.RemoteURL()
	if err != nil {
		t.Fatal(err)
	}
	if got != bare {
		t.Errorf("RemoteURL = %q, want %q", got, bare)
	}
}

func TestRemoteURLWithoutOriginReturnsError(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")

	if _, err := repo.RemoteURL(); err == nil {
		t.Fatal("want an error when no origin is configured, got nil")
	}
}

func TestRevParseReturnsFullHash(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")

	got, err := repo.RevParse("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 40 {
		t.Errorf("RevParse = %q, want a 40 character hash", got)
	}
}

func TestAheadAndBehindInSyncIsZero(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	addOrigin(t, repo)

	ahead, behind, err := repo.AheadAndBehind("origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if ahead != 0 || behind != 0 {
		t.Errorf("ahead, behind = %d, %d; want 0, 0", ahead, behind)
	}
}

func TestAheadAndBehindCountsLocalCommits(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	addOrigin(t, repo)
	commitFile(t, repo, ".vimrc", "set nocompatible\n")

	ahead, behind, err := repo.AheadAndBehind("origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if ahead != 1 || behind != 0 {
		t.Errorf("ahead, behind = %d, %d; want 1, 0", ahead, behind)
	}
}

func TestAheadAndBehindCountsRemoteCommitsAfterFetch(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	bare := addOrigin(t, repo)
	commitViaClone(t, bare, ".inputrc", "set editing-mode vi\n")
	mustGit(t, repo.Path, "fetch", "origin")

	ahead, behind, err := repo.AheadAndBehind("origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if ahead != 0 || behind != 1 {
		t.Errorf("ahead, behind = %d, %d; want 0, 1", ahead, behind)
	}
}

func TestAheadAndBehindWithUnknownRefReturnsError(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")

	if _, _, err := repo.AheadAndBehind("origin/main"); err == nil {
		t.Fatal("want an error for a missing upstream ref, got nil")
	}
}

func TestLogReturnsSubjectsNewestFirst(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, "first.txt", "1\n")
	commitFile(t, repo, "second.txt", "2\n")

	got, err := repo.Log(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("Log returned %d commits, want 2", len(got))
	}
	if got[0].Subject != "add second.txt" {
		t.Errorf("Log[0].Subject = %q, want %q", got[0].Subject, "add second.txt")
	}
	if len(got[0].Hash) < 7 {
		t.Errorf("Log[0].Hash = %q, want an abbreviated hash", got[0].Hash)
	}
}

func TestLogRangeListsOnlyIncomingCommits(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	bare := addOrigin(t, repo)
	was, err := repo.RevParse("origin/main")
	if err != nil {
		t.Fatal(err)
	}
	commitViaClone(t, bare, ".inputrc", "set editing-mode vi\n")
	mustGit(t, repo.Path, "fetch", "origin")

	got, err := repo.LogRange(was, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("LogRange returned %d commits, want 1: %+v", len(got), got)
	}
	if got[0].Subject != "add .inputrc" {
		t.Errorf("Subject = %q, want %q", got[0].Subject, "add .inputrc")
	}
}

func TestLogRangeWithNothingIncomingIsEmpty(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	addOrigin(t, repo)
	head, err := repo.RevParse("HEAD")
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.LogRange(head, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("LogRange = %+v, want empty", got)
	}
}

func TestDiffShowsChangeForModifiedFile(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, ".zshrc", "export A=2\n")

	got, err := repo.Diff(".zshrc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "-export A=1") || !strings.Contains(got, "+export A=2") {
		t.Errorf("Diff missing the change:\n%s", got)
	}
}

func TestDiffIncludesStagedChanges(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, ".zshrc", "export A=2\n")
	mustGit(t, repo.Path, "add", "--", ".zshrc")

	got, err := repo.Diff(".zshrc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "+export A=2") {
		t.Errorf("Diff missing the staged change:\n%s", got)
	}
}

func TestAddStagesOnlyTheGivenPaths(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	writeFile(t, repo.Path, ".zshrc", "export A=2\n")
	writeFile(t, repo.Path, ".untouched", "leave me\n")

	if err := repo.Add(".zshrc"); err != nil {
		t.Fatal(err)
	}

	entries := statusOf(t, repo)
	if got := findStatus(t, entries, ".zshrc"); !got.Staged() {
		t.Error(".zshrc Staged() = false, want true")
	}
	if got := findStatus(t, entries, ".untouched"); got.Staged() {
		t.Error(".untouched was staged, want it left alone")
	}
}

func TestIsRebasingIsFalseInACleanRepo(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")

	if repo.IsRebasing() {
		t.Error("IsRebasing = true, want false")
	}
}

func TestConflictedPathsIsEmptyWithoutAMerge(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")

	got, err := repo.ConflictedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("ConflictedPaths = %q, want empty", got)
	}
}

func TestConflictedPathsListsUnmergedFiles(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	bare := addOrigin(t, repo)
	commitViaClone(t, bare, ".zshrc", "export A=from-remote\n")
	writeFile(t, repo.Path, ".zshrc", "export A=from-local\n")
	mustGit(t, repo.Path, "add", "--", ".zshrc")
	mustGit(t, repo.Path, "commit", "-m", "local change")
	mustGit(t, repo.Path, "fetch", "origin")
	// The merge is expected to fail, so its exit code is deliberately ignored.
	_ = exec.Command("git", "-C", repo.Path, "merge", "origin/main").Run()

	got, err := repo.ConflictedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != ".zshrc" {
		t.Errorf("ConflictedPaths = %q, want [.zshrc]", got)
	}
}

func TestDiffStatSummarisesRange(t *testing.T) {
	repo := newRepo(t)
	commitFile(t, repo, ".zshrc", "export A=1\n")
	before, err := repo.RevParse("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, repo, ".vimrc", "set nocompatible\n")

	got, err := repo.DiffStat(before, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ".vimrc") {
		t.Errorf("DiffStat missing .vimrc:\n%s", got)
	}
}
