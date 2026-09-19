package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileStatus is one entry from git status.
// X is the index column and Y is the worktree column from --porcelain.
type FileStatus struct {
	X, Y      byte
	Path      string
	OldPath   string // the previous path of a rename or copy
	Untracked bool
	IsBinary  bool
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

func isBinary(path string) bool {
	// If the file doesn't exist, we just consider it not binary.
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	// Copy what Git does to detect if a file is binary - read the first 8000 bytes and check for a
	// NUL byte.
	buffer := make([]byte, 8000)
	n, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	return bytes.IndexByte(buffer[:n], 0) >= 0
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
	return parseStatus(repo.Path, out)
}

func parseStatus(root string, out string) ([]FileStatus, error) {
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

		filePath := record[3:]

		entry := FileStatus{
			X:    record[0],
			Y:    record[1],
			Path: filePath,
		}

		entry.Untracked = entry.X == '?' && entry.Y == '?'

		entry.IsBinary = !entry.Deleted() && isBinary(filepath.Join(root, filePath))

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
