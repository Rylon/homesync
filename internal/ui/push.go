package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Rylon/homesync/internal/git"
	"github.com/Rylon/homesync/internal/validate"
)

// The push screen works in three modes: browsing the file list, composing a commit
// message, and reporting files that failed validation.
type pushMode int

const (
	pushBrowsing pushMode = iota
	pushComposingCommit
	pushProblems
)

// pushRow is one selectable line in the file list.
type pushRow struct {
	file    git.FileStatus
	heading string // set when the row is a section heading
}

// pushState is used to track the state of the push screen, and also tracks the current
// file selections, so we can restore them if the user switches screens and comes back, or
// if the model is reloaded.
type pushState struct {
	mode     pushMode
	rows     []pushRow
	cursor   int
	selected map[string]bool

	message  textinput.Model
	problems []validate.Problem
	diff     string

	ready bool
}

// reconciles the push screen's row list with the latest fileGroups list, and attempts to preserve
// the current cursor position and any selections.
func (state *pushState) reconcile(groups fileGroups) {
	// Initialises the commit message input with placeholder text.
	if !state.ready {
		state.message = textinput.New()
		state.message.Placeholder = "Why was this change necessary?"
		state.message.CharLimit = 0
		state.ready = true
	}

	state.rows = nil

	if len(groups.Changed) > 0 {
		state.rows = append(state.rows, pushRow{heading: "CHANGED"})
		for _, file := range groups.Changed {
			state.rows = append(state.rows, pushRow{file: file})
		}
	}

	if len(groups.New) > 0 {
		state.rows = append(state.rows, pushRow{heading: "NEW"})
		for _, file := range groups.New {
			state.rows = append(state.rows, pushRow{file: file})
		}
	}

	// Reconstruct the selection map from the current file list, this makes sure there's
	// no nil values (avoids a possible panic on writing to a nil map), and also
	// preserves any existing selections the user has already made.
	selected := map[string]bool{}
	for _, row := range state.rows {
		if row.heading == "" && state.selected[row.file.Path] {
			selected[row.file.Path] = true
		}
	}
	state.selected = selected

	state.checkAndFixCursor(1)
}

// checkAndFixCursor is used after refreshing, and makes sure the cursor is still within
// the row list, and is still pointing at an actual file, not a header like "CHANGED".
func (state *pushState) checkAndFixCursor(direction int) {
	state.cursor = clampCursor(state.cursor, len(state.rows))

	for state.onHeading() {
		state.cursor += direction
	}

	// If we get past either the start or end of the list and find ourselves on a heading,
	// we reverse direction to find the nearest non-heading row.
	if state.cursor < 0 || state.cursor >= len(state.rows) {
		state.cursor = clampCursor(state.cursor, len(state.rows))
		for state.onHeading() {
			state.cursor -= direction
		}
	}
}

// onHeading reports whether the cursor is inside the list and on a heading row.
func (state pushState) onHeading() bool {
	return state.cursor >= 0 && state.cursor < len(state.rows) && state.rows[state.cursor].heading != ""
}

// currentFile returns the file under the cursor. `checkAndFixCursor` runs after every refresh
// to ensure the cursor remains within the bounds of the list, and on an actual file row.
func (state pushState) currentFile() (git.FileStatus, bool) {
	if len(state.rows) == 0 {
		return git.FileStatus{}, false
	}
	return state.rows[state.cursor].file, true
}

// selectedPaths builds a slice of the currently selected paths.
func (state pushState) selectedPaths() []string {
	var paths []string
	for _, row := range state.rows {
		if state.selected[row.file.Path] {
			paths = append(paths, row.file.Path)
		}
	}
	return paths
}

// diffMsg carries a loaded diff for whichever file the cursor is on.
type diffMsg struct {
	path string
	body string
	err  error
}

// handlePushKey deals with keypresses for the push screen, and the different modes it can be in.
func (model Model) handlePushKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch model.push.mode {
	case pushComposingCommit:
		return model.handlePushComposingCommitKey(msg, key)

	case pushProblems:
		if key == "esc" || key == "enter" {
			model.push.mode = pushBrowsing
			model.push.problems = nil
		}
		return model, nil
	}

	switch key {
	case "esc", "q":
		model.screen = screenDashboard
		return model, nil

	case "up", "k":
		if model.push.cursor > 0 {
			model.push.cursor--
			model.push.checkAndFixCursor(-1)
		}
		return model, model.loadDiff()

	case "down", "j":
		if model.push.cursor < len(model.push.rows)-1 {
			model.push.cursor++
			model.push.checkAndFixCursor(1)
		}
		return model, model.loadDiff()

	// Spacebar selects/deselects but the underlying library returns "space" for `KeyPressMsg.String()`.
	case "space":
		if file, ok := model.push.currentFile(); ok {
			model.push.selected[file.Path] = !model.push.selected[file.Path]
		}
		return model, nil

	// "select all" - selects all changed files
	case "a":
		for _, row := range model.push.rows {
			if row.heading == "" && !row.file.Untracked {
				model.push.selected[row.file.Path] = true
			}
		}
		model.notice = "selected all changed files, (untracked files are ignored)"
		return model, nil

	// "none" - deselects all files
	case "n":
		model.push.selected = map[string]bool{}
		model.notice = ""
		return model, nil

	// "commit" - opens the commit message editor if at least one file is selected.
	case "c":
		if len(model.push.selectedPaths()) == 0 {
			model.notice = "select at least one file first"
			return model, nil
		}

		model.push.mode = pushComposingCommit
		model.push.message.SetValue("")
		return model, model.push.message.Focus()

	// "push" - triggers the `git push` command.
	case "P":
		model.notice = ""
		return model, model.execGit("push", "push")

	// "refresh" - refreshes the state of the castle and homedir.
	case "r":
		return model.reload()
	}

	return model, nil
}

// loadDiff builds a command to generate a diff for the currently highlighted file.
// The returned command runs on its own goroutine, so a slow `git diff` never blocks the UI.
func (model Model) loadDiff() tea.Cmd {
	// returns nil when on a heading, or if the list is empty, so the diff panel shows "no file selected"
	// rather than the previous file's diff.
	file, ok := model.push.currentFile()
	if !ok {
		return nil
	}

	repo := model.repo
	abs := model.absPath(file.Path)

	return func() tea.Msg {
		// untracked files have no diff, so we just read the file contents, capped to 64KB.
		if file.Untracked {
			data, err := readCapped(abs)
			return diffMsg{path: file.Path, body: data, err: err}
		}
		// otherwise, we build a full diff from Git itself
		body, err := repo.Diff(file.Path)
		return diffMsg{path: file.Path, body: body, err: err}
	}
}

// handlePushComposingCommitKey handles key presses on the commit message composition screen.
func (model Model) handlePushComposingCommitKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {

	case "esc":
		model.push.mode = pushBrowsing
		model.push.message.Blur()
		return model, nil

	case "enter":
		message := strings.TrimSpace(model.push.message.Value())
		if message == "" {
			model.notice = "please enter a commit message"
			return model, nil
		}

		return model.commitSelectedFiles(message)
	}

	var cmd tea.Cmd
	model.push.message, cmd = model.push.message.Update(msg)
	return model, cmd
}

// commitSelectedFiles runs validation on the selected files, then stages and commits.
// Note that validation is only supported by a handleful of file types right now,
// and is basic, just to handle some of the common issues, like incomplete JSON, or
// merge conflict markers.
func (model Model) commitSelectedFiles(message string) (tea.Model, tea.Cmd) {
	paths := model.push.selectedPaths()

	// We need the absolute paths, not the repo-relative ones.
	absolute := make([]string, len(paths))
	for index, path := range paths {
		absolute[index] = model.absPath(path)
	}

	// Run validation on the selected files, and if any fail, abort the commit, show the problems to the user.
	if problems := validate.CheckAll(absolute); len(problems) > 0 {
		model.push.problems = problems
		model.push.mode = pushProblems
		return model, nil
	}

	// This triggers the `git add` for each file.
	if err := model.repo.Add(paths...); err != nil {
		model.err = err
		model.push.mode = pushBrowsing
		return model, nil
	}

	model.push.mode = pushBrowsing
	model.push.message.Blur()

	// Runs the actual `git commit`, execGit hands control to the terminal, so the user's own
	// Git config is used (name, commit signing, etc).
	return model, model.execGit("commit", "commit", "-m", message)
}

// pushExecDone reports how `git commit` or `git push` went. We refresh after either,
// so the UI stays current.
func (model Model) pushExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		model.err = fmt.Errorf("%s failed: %w", msg.label, msg.err)
		if msg.label == "push" {
			model.notice = "if origin has moved ahead, you may need to pull first."
		}

		return model.reload()
	}

	model.err = nil
	switch msg.label {
	case "commit":
		model.notice = "committed. select the next file(s) to commit, or press P to push to origin."

	case "push":
		model.notice = "pushed to origin."
	}

	return model.reload()
}

// viewPush draws the push screen, with the file list, a sidepanel containing a diff, and
// the commit message input below.
func (model Model) viewPush() string {
	if model.push.mode == pushProblems {
		return model.viewPushProblems()
	}

	// Split the content width between the panels, leaving room for the divider.
	available := model.contentWidth() - lipgloss.Width(divider)
	fileListWidth := available / 2
	diffWidth := available - fileListWidth

	var hints string

	if model.push.mode == pushComposingCommit {
		hints = model.help([2]string{"enter", "commit"}, [2]string{"esc", "cancel"})
	} else {
		hints = model.help(
			[2]string{"↑/↓", "move"},
			[2]string{"space", "select"},
			[2]string{"a", "all changed"},
			[2]string{"n", "none"},
			[2]string{"c", "commit"},
			[2]string{"P", "publish"},
			[2]string{"esc", "back"},
		)
	}

	// The chrome is the header and footer containing help hints, we need to check the actual
	// height, since the help row can be multiple lines.
	reserved := chromeOverhead + 1 + lipgloss.Height(hints)

	// notices and errors each take up two lines, so include them too.
	if model.notice != "" {
		reserved += 2
	}
	if model.err != nil {
		reserved += 2
	}

	var composeBlock []string

	if model.push.mode == pushComposingCommit {
		// The prompt text is part of the line, so we need to accomodate for it when calculating the width.
		model.push.message.SetWidth(model.contentWidth() - lipgloss.Width(model.push.message.Prompt))

		composeBlock = []string{
			"",
			headingStyle.Render(trimRight(fmt.Sprintf("Commit message for %s:",
				plural(len(model.push.selectedPaths()), "file")), model.contentWidth())),
			model.push.message.View(),
		}

		reserved += len(composeBlock)
	}

	// set a minimum panel height of 5, so diffs have room for at least a title, and a few lines
	// to give context.
	panelHeight := max(model.height-reserved, 5)

	fileList := model.renderPushFileList(fileListWidth, panelHeight)
	diff := model.renderPushDiff(diffWidth, panelHeight)

	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(fileListWidth).MaxWidth(fileListWidth).Render(fileList),
		divider,
		lipgloss.NewStyle().Width(diffWidth).MaxWidth(diffWidth).Render(diff),
	)

	parts := []string{panels}
	parts = append(parts, composeBlock...)
	parts = append(parts, "", hints)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderPushFileList draws the file list, the cursor, and the checkboxes for file selection.
func (model Model) renderPushFileList(width, height int) string {
	if len(model.push.rows) == 0 {
		return okStyle.Render("nothing to commit")
	}

	// A heading costs two lines and a file one, so a row index is not a line index. Note where
	// the cursor lands as we go, so the window below can keep it on screen.
	cursorLine := 0
	var lines []string

	for index, row := range model.push.rows {

		// headings don't need a cursor or checkboxes
		if row.heading != "" {
			style := warnStyle
			if row.heading == "NEW" {
				style = newStyle
			}
			lines = append(lines, "", style.Render(row.heading))
			continue
		}

		// selection checkboxes
		box := "[ ]"
		if model.push.selected[row.file.Path] {
			box = "[x]"
		}

		// cursor marker
		marker := "  "
		style := valueStyle
		if index == model.push.cursor {
			marker = "> "
			style = selectedStyle
			cursorLine = len(lines)
		}

		label := trimLeft(row.file.Path, width-8)
		lines = append(lines, marker+subtleStyle.Render(box)+" "+style.Render(label))
	}

	// The "Files" heading stays put and the list scrolls under it, so it costs one row.
	visibleFiles := scrollingFileList(lines, cursorLine, height-1)

	return lipgloss.JoinVertical(lipgloss.Left, append([]string{headingStyle.Render("Files")}, visibleFiles...)...)
}

// scrollWindow returns at most height lines, centred on the cursor line so it stays on screen
// however far down the list it moves. Without this the list is cut off at the bottom, and the
// cursor walks out of view while the spacebar still selects whatever it is sitting on.
// scrollingFileList handles if the file list is bigger than the available height,
// allowing the user to scroll through the list.
func scrollingFileList(lines []string, cursorLine, height int) []string {
	if height < 1 {
		height = 1
	}
	if len(lines) <= height {
		return lines
	}

	// The viewport is centered on the cursor when possible, but the cursor is allowed to scroll
	// to the very top/bottom of the list, so we don't end up with a lot of blank lines when
	// reaching the outer ends of the list.
	start := cursorLine - height/2
	if start < 0 {
		start = 0
	}
	if start > len(lines)-height {
		start = len(lines) - height
	}

	return lines[start : start+height]
}

// renderPushDiff draws the diff for the highlighted file, or its contents when the file is
// new and hasn't been committed previously.
func (model Model) renderPushDiff(width, height int) string {
	file, ok := model.push.currentFile()
	if !ok {
		return subtleStyle.Render("no file selected")
	}

	label := file.Path
	suffix := ""
	if file.Untracked {
		suffix = "  (untracked)"
	}
	title := headingStyle.Render(trimLeft(label, width-lipgloss.Width(suffix))) + newStyle.Render(suffix)

	// diffs are generated asynchronously...
	if model.push.diff == "" {
		return lipgloss.JoinVertical(lipgloss.Left, title, "", subtleStyle.Render("loading..."))
	}

	// The title and the blank line under it take two of the pane's rows.
	body := renderDiff(model.push.diff, width, height-2)
	return lipgloss.JoinVertical(lipgloss.Left, title, "", body)
}

// viewPushProblems explains why something couldn't be committed.
func (model Model) viewPushProblems() string {
	lines := []string{
		errStyle.Render(trimRight("These files did not pass validation, so couldn't be committed.", model.contentWidth())),
		subtleStyle.Render(trimRight("Validation checks basic syntax for support files, and looks for merge conflict markers. Please fix the errors below, then press r to refresh..", model.contentWidth())),
		"",
	}

	for _, problem := range model.push.problems {
		lines = append(lines, errStyle.Render(trimRight("  "+problem.Err.Error(), model.contentWidth())))
	}

	lines = append(lines, "", model.help([2]string{"enter", "back"}))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
