package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Commit struct {
	Hash    string
	Subject string // just the first line of the commit message.
}

// Branch returns the currently checked out branch name.
func (r *Repo) Branch() (string, error) {
	out, err := r.run("rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

// RemoteURL returns the origin URL. It errors when no origin is configured,
// because a castle without one cannot be pushed or pulled.
func (r *Repo) RemoteURL() (string, error) {
	out, err := r.run("config", "--get", "remote.origin.url")
	if err != nil {
		return "", fmt.Errorf("no origin remote configured for %s", r.Path)
	}

	return strings.TrimSpace(out), nil
}

// RevParse resolves a ref to its full hash.
func (r *Repo) RevParse(ref string) (string, error) {
	out, err := r.run("rev-parse", ref)
	return strings.TrimSpace(out), err
}

// AheadAndBehind counts commits that diverge from upstream. Ahead means local
// commits not on upstream. Behind means upstream commits we've not pulled yet.
func (r *Repo) AheadAndBehind(upstream string) (ahead, behind int, err error) {
	out, err := r.run("rev-list", "--left-right", "--count", upstream+"...HEAD")
	if err != nil {
		return 0, 0, err
	}

	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected number of fields from `rev-list`, expected 2, but got %q", out)
	}

	if behind, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, err
	}

	if ahead, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, err
	}

	return ahead, behind, nil
}

// This fetches the commit log up to the specified limit, with the newest commits first, and returns
// a slice of Commit structs.
func (r *Repo) Log(limit int) ([]Commit, error) {
	return r.parseLog("log", "--oneline", "--no-decorate", "-n", strconv.Itoa(limit))
}

// This fetches the commit logs in the range from..to, and returns a slice of Commit structs.
// When fetching changes from upstream, it's used to track what changes came in from the remote,
// compared to which commits we had already made locally which haven't been pushed yet.
// Without this, we'd not be able to tell our own local commits apart from the upstream ones.
func (r *Repo) LogRange(from, to string) ([]Commit, error) {
	return r.parseLog("log", "--oneline", "--no-decorate", from+".."+to)
}

// Parses the log lines in the format `<abbrev-hash> <subject>` from Log or LogRange (--oneline format)
// builds a slice of Commit structs for each.
func (r *Repo) parseLog(args ...string) ([]Commit, error) {
	out, err := r.run(args...)
	if err != nil {
		return nil, err
	}

	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		hash, subject, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		commits = append(commits, Commit{Hash: hash, Subject: subject})
	}

	return commits, nil
}

// Diff returns the combined staged and unstaged diff for a given path, so we see the whole change.
func (r *Repo) Diff(path string) (string, error) {
	return r.run("diff", "HEAD", "--", path)
}

// DiffStat summarises the change between two refs.
func (r *Repo) DiffStat(from, to string) (string, error) {
	return r.run("diff", "--stat", from+".."+to)
}

// Add stages the given file paths explicitly, avoiding any use of -A, or -u, which
// would result in unintended changes being staged along with the intended ones.
func (r *Repo) Add(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := r.run(append([]string{"add", "--"}, paths...)...)
	return err
}

// IsRebasing reports whether a rebase is in progress. A rebase that has conflicts
// ends up leaving marker text in the affected files, so it's a problem.
func (r *Repo) IsRebasing() bool {
	for _, dir := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(r.Path, ".git", dir)); err == nil {
			return true
		}
	}
	return false
}

// ConflictedPaths lists files with unresolved merge conflicts.
func (r *Repo) ConflictedPaths() ([]string, error) {
	out, err := r.run("diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
