package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/Rylon/homesync/internal/castle"
	"github.com/Rylon/homesync/internal/git"
	"github.com/Rylon/homesync/internal/link"
)

// a representative castle with some sample files and links, to test the layout.
func exampleCastleModel(width, height int) Model {
	testCastle := castle.Castle{
		Name: "dotfiles",
		Root: "/home/u/.homesick/repos/dotfiles",
	}
	model := Model{
		homeDir: "/home/u",
		castles: []castle.Castle{testCastle},
		castle:  testCastle,
		roots:   []string{testCastle.Root},
		branch:  "main",
		remote:  "git@github.com:example/dotfiles.git",
	}
	model.width, model.height = width, height
	model.files = []git.FileStatus{
		{X: ' ', Y: 'M', Path: "home/.exampleapp/settings.json"},
		{X: ' ', Y: 'M', Path: "home/.config/exampletool/config"},
		{X: '?', Y: '?', Path: "home/.some/deeply/nested/untracked/file.txt", Untracked: true},
	}
	model.groups = groupFiles(model.files)
	model.actions = []link.Action{
		{Kind: link.Identical, Rel: ".zshrc"},
		{Kind: link.Create, Rel: ".example.toml", Destination: "/home/u/.example.toml", Source: testCastle.Home() + "/.example.toml"},
		{Kind: link.Conflict, Rel: ".vimrc", Destination: "/home/u/.vimrc"},
		{Kind: link.Refused, Rel: ".homesick/repos/dotfiles/stray", Reason: "destination is inside the castle at " + testCastle.Root},
	}
	model.health = summariseLinks(model.actions)
	model.push.reconcile(model.groups)
	model.push.diff = strings.Repeat("diff --git a/home/.exampleapp/settings.json b/home/.exampleapp/settings.json\n+a fairly long added line that should be clipped to the pane\n", 12)
	model.relink.reconcile(model.actions)
	return model
}

// frameSize measures the rendered frame, so we can check nothing overflowed.
func frameSize(content string) (width, height int) {
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		if lineWidth := lipgloss.Width(line); lineWidth > width {
			width = lineWidth
		}
	}

	return width, len(lines)
}

// Runs through various terminal sizes, and makes sure the screens never exceed them,
// and that they resize properly.
func TestScreenResizingFitsVariousTerminalSizes(t *testing.T) {
	sizes := [][2]int{{80, 24}, {100, 30}, {60, 20}, {140, 40}, {200, 50}}

	screens := []struct {
		name  string
		setup func(*Model)
	}{
		{"dashboard", func(model *Model) { model.screen = screenDashboard }},
		{"push", func(model *Model) { model.screen = screenPush }},
		{"push composing", func(model *Model) { model.screen = screenPush; model.push.mode = pushComposingCommit }},
		{"push problems", func(model *Model) {
			model.screen = screenPush
			model.push.mode = pushProblems
		}},
		{"pull gate", func(model *Model) { model.screen = screenPull }},
		{"relink", func(model *Model) { model.screen = screenRelink }},
		{"relink confirm", func(model *Model) { model.screen = screenRelink; model.relink.confirm = true }},
		{"picker", func(model *Model) { model.screen = screenPicker }},
	}

	for _, size := range sizes {
		width, height := size[0], size[1]

		for _, screen := range screens {
			model := exampleCastleModel(width, height)
			screen.setup(&model)

			model.notice = "committed. select the next group, or press P to publish."

			gotW, gotH := frameSize(model.View().Content)

			if gotW > width {
				t.Errorf("%s at %dx%d: frame width %d exceeds terminal width %d", screen.name, width, height, gotW, width)
			}

			if gotH > height {
				t.Errorf("%s at %dx%d: frame height %d exceeds terminal height %d", screen.name, width, height, gotH, height)
			}
		}
	}
}

// The help row is the widest fixed element, so we need to make sure it wraps to multiple rows,
// rather than being truncated.
func TestHelpRowWrapsToWidth(t *testing.T) {
	pairs := [][2]string{
		{"↑/↓", "move"}, {"space", "select"}, {"a", "all changed"},
		{"n", "none"}, {"c", "commit"}, {"P", "publish"}, {"esc", "back"},
	}

	for _, width := range []int{40, 60, 80, 200} {
		got := helpWidth(width, pairs...)
		for index, line := range strings.Split(got, "\n") {
			if lineWidth := lipgloss.Width(line); lineWidth > width {
				t.Errorf("width %d: help line %d is %d wide:\n%s", width, index, lineWidth, line)
			}
		}
	}
}

func TestHelpRowShowsAllOptionsWhenWrapped(t *testing.T) {
	pairs := [][2]string{{"a", "all changed"}, {"P", "publish"}, {"esc", "back"}}

	got := helpWidth(20, pairs...)

	for _, want := range []string{"all changed", "publish", "back"} {
		if !strings.Contains(got, want) {
			t.Errorf("wrapped help lost %q:\n%s", want, got)
		}
	}
}
