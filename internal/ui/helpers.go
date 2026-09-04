package ui

import (
	"fmt"
	"path/filepath"
	"strings"
)

// plural is a naive pluraliser, it just adds an "s" to the end if the count isn't 1, so it
// won't handle irregular plurals, but it's good enough for our needs :)
func plural(count int, word string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", count, word)
}

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
