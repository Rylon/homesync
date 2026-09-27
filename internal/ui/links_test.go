package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReloadDoesNotPlanLinksForFilesThatGitIgnores(t *testing.T) {
	model := castleBehindOrigin(t)
	home := filepath.Join(model.castle.Root, "home")
	for rel, content := range map[string]string{".gitignore": ".DS_Store\n", ".DS_Store": "finder\n", ".zshrc": "export A=1\n"} {
		if err := os.MkdirAll(home, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, cmd := model.reload()
	msg := cmd().(loadedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}

	planned := map[string]bool{}
	for _, action := range msg.actions {
		planned[action.Rel] = true
	}
	if planned[".DS_Store"] {
		t.Error("planned a link for .DS_Store, which git ignores")
	}
	if !planned[".zshrc"] {
		t.Errorf("did not plan a link for .zshrc, got %v", planned)
	}
}
