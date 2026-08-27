package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckAcceptsValidJSON(t *testing.T) {
	path := write(t, "settings.json", `{"model": "opus", "nested": {"a": [1, 2]}}`)

	if err := Check(path); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}
}

func TestCheckRejectsTruncatedJSON(t *testing.T) {
	path := write(t, "settings.json", `{"model": "opus"`)

	err := Check(path)
	if err == nil {
		t.Fatal("want an error for truncated JSON, got nil")
	}
	if !strings.Contains(err.Error(), "settings.json") {
		t.Errorf("error = %q, want it to name the file", err)
	}
}

func TestCheckRejectsJSONWithTrailingComma(t *testing.T) {
	path := write(t, "settings.json", `{"a": 1,}`)

	if err := Check(path); err == nil {
		t.Fatal("want an error for a trailing comma, got nil")
	}
}

// An editor that saved half a file leaves conflict markers or partial syntax.
func TestCheckRejectsJSONWithConflictMarkers(t *testing.T) {
	path := write(t, "settings.json", "{\n<<<<<<< HEAD\n\"a\": 1\n=======\n\"a\": 2\n>>>>>>> other\n}\n")

	if err := Check(path); err == nil {
		t.Fatal("want an error for conflict markers, got nil")
	}
}

func TestCheckAcceptsValidShell(t *testing.T) {
	path := write(t, "aliases.sh", "alias ll='ls -la'\nif [ -n \"$HOME\" ]; then\n  echo hi\nfi\n")

	if err := Check(path); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}
}

func TestCheckRejectsShellWithUnclosedBlock(t *testing.T) {
	path := write(t, "aliases.sh", "if [ -n \"$HOME\" ]; then\n  echo hi\n")

	err := Check(path)
	if err == nil {
		t.Fatal("want an error for an unclosed if, got nil")
	}
	if !strings.Contains(err.Error(), "aliases.sh") {
		t.Errorf("error = %q, want it to name the file", err)
	}
}

func TestCheckAcceptsBashExtension(t *testing.T) {
	path := write(t, "functions.bash", "greet() { echo hi; }\n")

	if err := Check(path); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}
}

func TestCheckRejectsBrokenBashExtension(t *testing.T) {
	path := write(t, "functions.bash", "greet() { echo hi;\n")

	if err := Check(path); err == nil {
		t.Fatal("want an error for an unclosed function, got nil")
	}
}

// Most dotfiles have no checker. They must pass rather than block a commit.
func TestCheckIgnoresUnknownExtension(t *testing.T) {
	path := write(t, "starship.toml", "this is not [valid toml")

	if err := Check(path); err != nil {
		t.Errorf("Check = %v, want nil for an unchecked type", err)
	}
}

func TestCheckIgnoresMissingFile(t *testing.T) {
	// A deleted file is a legitimate part of a commit.
	path := filepath.Join(t.TempDir(), "gone.json")

	if err := Check(path); err != nil {
		t.Errorf("Check = %v, want nil for a deleted file", err)
	}
}

func TestCheckAllReportsEveryFailure(t *testing.T) {
	good := write(t, "good.json", `{"a": 1}`)
	bad := write(t, "bad.json", `{"a":`)
	alsoBad := write(t, "broken.sh", "while true; do\n")

	problems := CheckAll([]string{good, bad, alsoBad})

	if len(problems) != 2 {
		t.Fatalf("CheckAll returned %d problems, want 2: %v", len(problems), problems)
	}
	paths := problems[0].Path + " " + problems[1].Path
	if !strings.Contains(paths, "bad.json") || !strings.Contains(paths, "broken.sh") {
		t.Errorf("problems name %q, want bad.json and broken.sh", paths)
	}
}

func TestCheckAllOnAllValidFilesReturnsNothing(t *testing.T) {
	good := write(t, "good.json", `{"a": 1}`)

	if problems := CheckAll([]string{good}); len(problems) != 0 {
		t.Errorf("CheckAll = %v, want empty", problems)
	}
}
