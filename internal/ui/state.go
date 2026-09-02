package ui

import (
	"github.com/Rylon/homesync/internal/git"
	"github.com/Rylon/homesync/internal/link"
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

// linkSummary tallies a link plan so the dashboard, the relink screen and the
// pull report can show the correct counts, rather than needing to reconstruct them
// each time.
type linkSummary struct {
	Identical int
	Missing   int
	Conflicts int
	Refused   int
}

// Problems counts how many of the changes require a human decision, such as conflicts.
func (summary linkSummary) Problems() int {
	return summary.Missing + summary.Conflicts + summary.Refused
}

// summariseLinks counts a plan grouped by each Kind,. We count both types of symlink
// conflicts together, because the resolution is the same in both cases.
func summariseLinks(actions []link.Action) linkSummary {
	var summary linkSummary
	for _, action := range actions {
		switch action.Kind {

		case link.Identical:
			summary.Identical++

		case link.Create:
			summary.Missing++

		case link.Conflict, link.SymlinkConflict:
			summary.Conflicts++

		case link.Refused:
			summary.Refused++
		}
	}
	return summary
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
