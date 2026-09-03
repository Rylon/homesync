package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Rylon/homesync/internal/git"
)

// Generate a real spacebar KeyPressMsg, to simulate the spacebar for our tests.
func spaceKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

// Build a model with one changed file in a fake push screen.
func pushModelWithOneFile() Model {
	model := Model{screen: screenPush}
	model.push.selected = map[string]bool{}

	model.push.rows = []pushRow{
		{heading: "CHANGED"},
		{file: git.FileStatus{Path: "home/.exampleapp/settings.json", X: ' ', Y: 'M'}},
	}

	model.push.cursor = 1

	return model
}

// The next three are regression tests for selecting and deselecting files with spacebar.
// Originally we were checking for " " in the switch, but the underlying library sends
// a KeyPressMsg which returns "space" for String() instead, so we were never matching.

// First make sure the file is selected when we press space.
func TestSpaceSelectsTheHighlightedFile(t *testing.T) {
	model := pushModelWithOneFile()

	next, _ := model.handleKey(spaceKey())

	if !next.(Model).push.selected["home/.exampleapp/settings.json"] {
		t.Error("space did not select the highlighted file")
	}
}

// Then make sure the file is deselected when we press space again.
func TestSpaceDeselectsAnAlreadySelectedFile(t *testing.T) {
	model := pushModelWithOneFile()
	model.push.selected["home/.exampleapp/settings.json"] = true

	next, _ := model.handleKey(spaceKey())

	if next.(Model).push.selected["home/.exampleapp/settings.json"] {
		t.Error("space did not deselect the file")
	}
}
