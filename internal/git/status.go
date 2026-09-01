package git

import (
	"fmt"
	"strings"
)

// FileStatus is one entry from git status.
// X is the index column and Y is the worktree column from --porcelain.
type FileStatus struct {
	X, Y      byte
	Path      string
	OldPath   string // the previous path of a rename or copy
	Untracked bool
}

// Has this file been staged for commit?
func (status FileStatus) Staged() bool {
	return status.X != ' ' && status.X != '?'
}

// Has the file been modified in the worktree, but not yet staged for commit?
func (status FileStatus) Unstaged() bool {
	return status.Y != ' ' && status.Y != '?'
}

// The file has been deleted from the repo.
func (status FileStatus) Deleted() bool {
	return status.X == 'D' || status.Y == 'D'
}

// Status returns every changed and untracked path in the repo.
//
//   - `-uall` also lists files inside a new directory, otherwise we'd just see the new directory only,
//     and potentially miss new files inside it.
//   - `-z` separates records with NUL, which stops --porcelain quoting paths that contain spaces or quotes.
func (repo *Repo) Status() ([]FileStatus, error) {
	out, err := repo.run("status", "--porcelain=v1", "-uall", "-z")
	if err != nil {
		return nil, err
	}
	return parseStatus(out)
}

func parseStatus(out string) ([]FileStatus, error) {
	records := strings.Split(out, "\x00")

	var entries []FileStatus
	for i := 0; i < len(records); i++ {
		record := records[i]

		// The final NUL leaves an empty trailing record, so we need to skip that.
		if record == "" {
			continue
		}

		if len(record) < 4 {
			return nil, fmt.Errorf("status record %q is too short to hold a status and a path", record)
		}

		entry := FileStatus{
			X:    record[0],
			Y:    record[1],
			Path: record[3:],
		}
		entry.Untracked = entry.X == '?' && entry.Y == '?'

		// A rename or copy emits the old path as its own record. Consuming it
		// here keeps the following entries aligned.
		if entry.X == 'R' || entry.X == 'C' || entry.Y == 'R' || entry.Y == 'C' {
			if i+1 >= len(records) {
				return nil, fmt.Errorf("status record %q claims a rename, but no previous path follows", record)
			}
			i++
			entry.OldPath = records[i]
		}

		entries = append(entries, entry)
	}
	return entries, nil
}
