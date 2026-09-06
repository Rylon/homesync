package ui

import (
	"testing"

	"github.com/Rylon/homesync/internal/git"
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
