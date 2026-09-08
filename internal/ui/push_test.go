package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Rylon/homesync/internal/git"
)

// A typical file list, with two sections, and a file in each.
func fileListWithTwoSections() []pushRow {
	return []pushRow{
		{heading: "CHANGED"},
		{file: git.FileStatus{Path: "home/.zshrc"}},
		{heading: "NEW"},
		{file: git.FileStatus{Path: "home/.vimrc"}},
	}
}

// checkAndFixCursor makes sure the cursor doesn't end up broken when the file list changes from
// under it (during refresh).
func TestCheckAndFixCursorWorks(t *testing.T) {
	cases := []struct {
		name      string
		rows      []pushRow
		cursor    int
		direction int
		want      int
	}{
		{"already on a file, stays put", fileListWithTwoSections(), 1, 1, 1},
		{"on the first heading, moves down", fileListWithTwoSections(), 0, 1, 1},
		{"on the middle heading, moves down", fileListWithTwoSections(), 2, 1, 3},
		{"on the middle heading, moves up", fileListWithTwoSections(), 2, -1, 1},
		{"on the first heading with no file above, takes the first file below", fileListWithTwoSections(), 0, -1, 1},
		{"past the end, moves back to the last file", fileListWithTwoSections(), 99, 1, 3},
		{"before the start, moves back to the first file", fileListWithTwoSections(), -5, 1, 1},
		{"no rows at all returns zero", nil, 0, 1, 0},
		{"no rows at all returns zero when it was previously non-zero", nil, 7, 1, 0},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			state := pushState{rows: testCase.rows, cursor: testCase.cursor}

			state.checkAndFixCursor(testCase.direction)

			if state.cursor != testCase.want {
				t.Errorf("cursor = %d, want %d", state.cursor, testCase.want)
			}
		})
	}
}

// This shouldn't happen normally, but we want to make sure it's handled gracefully.
func TestCheckAndFixCursorHandlesATrailingHeading(t *testing.T) {
	state := pushState{
		rows: []pushRow{
			{heading: "CHANGED"},
			{file: git.FileStatus{Path: "home/.zshrc"}},
			{heading: "NEW"},
		},
		cursor: 2,
	}

	state.checkAndFixCursor(1)

	if state.cursor != 1 {
		t.Errorf("cursor = %d, want 1, the non-header entry", state.cursor)
	}
}

// reconcile should ensure the selection map is initialised and is non-nil, or attempts
// to write to it will cause a panic.
func TestReconcileMakesTheSelectionMapUsable(t *testing.T) {
	model := Model{screen: screenPush}
	if model.push.selected != nil {
		t.Fatal("we expect it to start nil, but it is not, so it's unsafe to proceed")
	}

	// bring in a pretend change to the filelist, and make sure the map is no longer nil
	model.push.reconcile(fileGroups{Changed: []git.FileStatus{modified("home/.zshrc")}})

	if model.push.selected == nil {
		t.Fatal("reconcile left the selection map nil, it should have been initialised")
	}

	// Make sure selection now works, and doesn't panic, by selecting the file we just added.
	next, _ := model.handleKey(spaceKey())
	if !next.(Model).push.selected["home/.zshrc"] {
		t.Error("spacebar did not select the new file after reconciling")
	}
}

func TestReconcilePreservesExistingSelections(t *testing.T) {
	state := pushState{selected: map[string]bool{"home/.zshrc": true}}

	state.reconcile(fileGroups{Changed: []git.FileStatus{modified("home/.zshrc")}})

	if !state.selected["home/.zshrc"] {
		t.Error("reconcile did not preserve the existing selections")
	}
}

// A committed file leaves the list, and its selection has to go with it, or the next commit
// would quietly pick it up again.
func TestReconcileDropsSelectionsForFilesThatAreGone(t *testing.T) {
	state := pushState{selected: map[string]bool{"home/.committed": true}}

	state.reconcile(fileGroups{Changed: []git.FileStatus{modified("home/.zshrc")}})

	if state.selected["home/.committed"] {
		t.Error("reconcile kept a selection for a file that is no longer listed")
	}
}

// human readable numbered lines, for better error messages if the scrolling tests fail
func numberedLines(count int) []string {
	lines := make([]string, count)
	for index := range lines {
		lines[index] = fmt.Sprintf("line %d", index)
	}
	return lines
}

// Make sure the cursor is always visible when scrolling through a longer list of files.
func TestScrollingFileListKeepsTheCursorVisible(t *testing.T) {
	cases := []struct {
		name       string
		lineCount  int
		cursorLine int
		height     int
		wantFirst  string
		wantLast   string
	}{
		{"shorter than the pane, no scrolling", 3, 1, 10, "line 0", "line 2"},
		{"cursor at the top, viewport starts at the top", 20, 0, 5, "line 0", "line 4"},
		{"cursor in the middle, viewport centers on it", 20, 10, 5, "line 8", "line 12"},
		{"cursor at the bottom, viewport ends at the bottom", 20, 19, 5, "line 15", "line 19"},
		{"cursor near the bottom, viewport stops at the end", 20, 18, 5, "line 15", "line 19"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			lines := numberedLines(testCase.lineCount)

			got := scrollingFileList(lines, testCase.cursorLine, testCase.height)

			if len(got) > testCase.height {
				t.Fatalf("returned %d lines, wanted at most %d", len(got), testCase.height)
			}
			if got[0] != testCase.wantFirst {
				t.Errorf("first line = %q, wanted %q", got[0], testCase.wantFirst)
			}
			if got[len(got)-1] != testCase.wantLast {
				t.Errorf("last line = %q, wanted %q", got[len(got)-1], testCase.wantLast)
			}
			if !slices.Contains(got, lines[testCase.cursorLine]) {
				t.Errorf("cursor line %q is not in the viewport %q", lines[testCase.cursorLine], got)
			}
		})
	}
}

// The viewport is already clamped, so we just want to make sure an invalid value is handled
// gracefully rather than crashing.
func TestScrollingFileListHandlesAnInvalidHeight(t *testing.T) {
	for _, height := range []int{0, -1, -7} {
		got := scrollingFileList(numberedLines(20), 10, height)

		if len(got) != 1 {
			t.Errorf("height %d returned %d lines, want 1", height, len(got))
		}
	}
}

// Tests that our scrolling filelist works properly, so the cursor is always visible within the viewport.
func TestFileListScrolling(t *testing.T) {

	// Add 20 changed and new files so we have a big test list to scroll through.
	var changed, added []git.FileStatus
	for index := range 20 {
		changed = append(changed, modified(fmt.Sprintf("home/.changed%02d", index)))
		added = append(added, untracked(fmt.Sprintf("home/.new%02d", index)))
	}

	model := Model{screen: screenPush, width: 80, height: 24}
	model.push.reconcile(fileGroups{Changed: changed, New: added})

	// Run through all our test files, and make sure the cursor is always on a file,
	// (not a heading), and is always visible within the viewport.
	for index, row := range model.push.rows {
		if row.heading != "" {
			continue
		}

		model.push.cursor = index

		// A five row pane against a forty two row list, to guarantee the scrolling viewport is needed.
		rendered := model.renderPushFileList(40, 5)

		if !strings.Contains(rendered, row.file.Path) {
			t.Errorf("cursor on row %d: %q is not on screen:\n%s", index, row.file.Path, rendered)
		}

		if !strings.Contains(rendered, ">") {
			t.Errorf("cursor on row %d: the marker is not on screen:\n%s", index, rendered)
		}
	}
}
