package ui

import (
	"path/filepath"
	"strings"
)

// tildePath renders paths within the $HOME folder shortened to ~, but returns
// the path unmodified if it's outside $HOME.
func (model Model) tildePath(path string) string {
	if rel, err := filepath.Rel(model.homeDir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}

// absPath turns a repo-relative path into an absolute one. Git accepts the
// relative form because every command runs with `-C <castle root>`, but the os
// package resolves against the working directory, which is wherever homesync
// was launched from.
func (model Model) absPath(rel string) string {
	return filepath.Join(model.castle.Root, rel)
}
