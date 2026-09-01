// Package validate checks a file's syntax before it is committed,
// it checks a few common file types just to make sure we're not accidentally
// committing malformed files that will break things when pulled.
package validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Problem represents a particular file and what is wrong with it.
type Problem struct {
	Path string
	Err  error
}

func (problem Problem) String() string {
	return problem.Err.Error()
}

// Check validates a given file. If the file extension has no checker defined,
// we assume it passes.
func Check(path string) error {

	// If the file no longer exists, we also treat it as if it passed.
	if _, err := os.Stat(path); err != nil {
		return nil
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return checkJSON(path)
	case ".sh", ".bash":
		return checkShell(path)
	}

	return nil
}

// CheckAll validates every path and returns one Problem per file, so the
// interface can show all the problems files at once. If a file has multiple
// problems, they won't all appear at once, the user can fix the problem,
// and the next problem will be shown (if any).
func CheckAll(paths []string) []Problem {
	var problems []Problem
	for _, path := range paths {
		if err := Check(path); err != nil {
			problems = append(problems, Problem{Path: path, Err: err})
		}
	}
	return problems
}

func checkJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Unmarshal into any, which rejects trailing data and trailing commas that
	// wouldn't show up as a problem just by unmarshaling into a map[string].
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("%s: invalid JSON: %w", filepath.Base(path), err)
	}

	return nil
}

func checkShell(path string) error {
	var stderr bytes.Buffer

	// Tells bash to read the syntax, but not execute it. Any syntax errors will be sent to stderr.
	cmd := exec.Command("bash", "-n", path)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s: invalid shell syntax: %s", filepath.Base(path), message)
	}

	return nil
}
