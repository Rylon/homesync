package ui

import (
	"testing"

	"github.com/Rylon/homesync/internal/git"
	"github.com/Rylon/homesync/internal/link"
)

func modified(path string) git.FileStatus {
	return git.FileStatus{X: ' ', Y: 'M', Path: path}
}

func staged(path string) git.FileStatus {
	return git.FileStatus{X: 'M', Y: ' ', Path: path}
}

func untracked(path string) git.FileStatus {
	return git.FileStatus{X: '?', Y: '?', Path: path, Untracked: true}
}

func paths(files []git.FileStatus) []string {
	out := make([]string, len(files))
	for index, file := range files {
		out[index] = file.Path
	}
	return out
}

func equal(first, second []string) bool {
	// Check if the length is the same first, so the array scan below is safe.
	if len(first) != len(second) {
		return false
	}

	// Check if each line is the same.
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}

	return true
}

func TestGroupSeparatesChangedFromNew(t *testing.T) {
	got := groupFiles([]git.FileStatus{
		modified("home/.zshrc"),
		untracked("home/.secret"),
		staged("home/.vimrc"),
	})

	if want := []string{"home/.zshrc", "home/.vimrc"}; !equal(paths(got.Changed), want) {
		t.Errorf("Changed = %q, want %q", paths(got.Changed), want)
	}

	if want := []string{"home/.secret"}; !equal(paths(got.New), want) {
		t.Errorf("New = %q, want %q", paths(got.New), want)
	}
}

func TestGroupOnCleanRepoIsEmpty(t *testing.T) {
	got := groupFiles(nil)

	if len(got.Changed) != 0 || len(got.New) != 0 {
		t.Errorf("Group = %+v, want both empty", got)
	}
}

func TestSummariseProperlyCountsEachKind(t *testing.T) {
	got := summariseLinks([]link.Action{
		{Kind: link.Identical},
		{Kind: link.Identical},
		{Kind: link.Create},
		{Kind: link.Conflict},
		{Kind: link.SymlinkConflict},
		{Kind: link.Refused},
	})

	if got.Identical != 2 {
		t.Errorf("Identical = %d, want 2", got.Identical)
	}

	if got.Missing != 1 {
		t.Errorf("Missing = %d, want 1", got.Missing)
	}

	// Both conflict kinds are reported together.
	if got.Conflicts != 2 {
		t.Errorf("Conflicts = %d, want 2", got.Conflicts)
	}

	if got.Refused != 1 {
		t.Errorf("Refused = %d, want 1", got.Refused)
	}
}

func TestSummariseOnFullyLinkedCastleReportsNoProblems(t *testing.T) {
	got := summariseLinks([]link.Action{{Kind: link.Identical}, {Kind: link.Identical}})

	if got.Problems() != 0 {
		t.Errorf("Problems = %d, want 0", got.Problems())
	}
}

func TestSummariseProblemsCountsMissingAndConflicts(t *testing.T) {
	got := summariseLinks([]link.Action{
		{Kind: link.Identical},
		{Kind: link.Create},
		{Kind: link.Conflict},
		{Kind: link.Refused},
	})

	if got.Problems() != 3 {
		t.Errorf("Problems = %d, want 3", got.Problems())
	}
}

// Make sure any local changes to tracked files block a pull, so they can be dealt with first,
// and aren't accidentally overwritten.
func TestGateForPullBlocksOnModifiedTrackedFile(t *testing.T) {
	got := gateForPull([]git.FileStatus{modified("home/.zshrc")})

	if !got.Blocked {
		t.Error("Blocked = false, want true")
	}

	if want := []string{"home/.zshrc"}; !equal(got.Blockers, want) {
		t.Errorf("Blockers = %q, want %q", got.Blockers, want)
	}
}

// The other half of the rule: untracked files should be ignored.
func TestGateForPullIgnoresUntrackedFiles(t *testing.T) {
	got := gateForPull([]git.FileStatus{
		untracked("home/.secret"),
		untracked("home/bin/local-tool"),
	})

	if got.Blocked {
		t.Errorf("Blocked = true with only untracked files, want false (blockers %q)", got.Blockers)
	}

	if len(got.Blockers) != 0 {
		t.Errorf("Blockers = %q, want empty", got.Blockers)
	}
}

func TestGateForPullBlocksOnStagedFile(t *testing.T) {
	got := gateForPull([]git.FileStatus{staged("home/.vimrc")})

	if !got.Blocked {
		t.Error("Blocked = false, want true")
	}
}

func TestGateForPullListsOnlyTheBlockingFiles(t *testing.T) {
	got := gateForPull([]git.FileStatus{
		untracked("home/.secret"),
		modified("home/.zshrc"),
		untracked("home/other"),
	})

	if !got.Blocked {
		t.Error("Blocked = false, want true")
	}

	if want := []string{"home/.zshrc"}; !equal(got.Blockers, want) {
		t.Errorf("Blockers = %q, want %q", got.Blockers, want)
	}
}

func TestGateForPullOnCleanRepoAllowsPull(t *testing.T) {
	got := gateForPull(nil)

	if got.Blocked {
		t.Error("Blocked = true on a clean repo, want false")
	}
}
