package ui

import (
	"github.com/Rylon/homesync/internal/git"
)

// State deals with the current state of the castle vs home, so the UI can present
// a summary of what is going on, and what actions the user might need to take.

// fileGroups splits the full FileStatus list into two groups, so we can
// see the headings "NEW" and "CHANGED" in the UI.
type fileGroups struct {
	Changed []git.FileStatus
	New     []git.FileStatus
}

func groupFiles(files []git.FileStatus) fileGroups {
	var groups fileGroups

	for _, file := range files {
		if file.Untracked {
			groups.New = append(groups.New, file)
		} else {
			groups.Changed = append(groups.Changed, file)
		}
	}

	return groups
}

// potentialPullBlockers are reasons that prevent us from running the `git pull` command, such as
// untracked files that might be overwritten, or uncommitted modifications that might be lost.
type potentialPullBlockers struct {
	Blocked  bool
	Blockers []string
}

// gateForPull goes through the potential blockers and decides whether it's safe to continue,
func gateForPull(files []git.FileStatus) potentialPullBlockers {
	var gate potentialPullBlockers
	for _, file := range files {
		if file.Untracked {
			continue
		}
		gate.Blockers = append(gate.Blockers, file.Path)
	}
	gate.Blocked = len(gate.Blockers) > 0
	return gate
}
