// Package ui is the Bubble Tea interface. It calls the various internal packages
// to figure out the state of the castle, the home dir, and what actions are needed,
// and deals with rendering the UI for all of that to the screen.
package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Rylon/homesync/internal/castle"
	"github.com/Rylon/homesync/internal/git"
	"github.com/Rylon/homesync/internal/link"
)

type screen int

const (
	screenDashboard screen = iota
	screenPush
	screenPull
	screenRelink
	screenPicker
)

// Model is the root model tracking the state of everything needed by each subcommand, so they
// can all share the same snapshots of the castle and home, rather than building it each time.
type Model struct {
	homeDir string
	castles []castle.Castle
	roots   []string

	castle  castle.Castle
	repo    *git.Repo
	subdirs []string
	linker  *link.Linker

	screen        screen
	width, height int

	// Refreshed wholesale by `reload`. Embedded, so `model.branch` and the
	// rest still read directly.
	snapshot

	loading bool
	notice  string
	err     error

	pickerCursor int

	push   pushState
	pull   pullState
	relink relinkState
}

// New builds the root Model for all the castles on a system, prompting the user to choose
// if they have more than one, or loading straight in if there's only one.
func New(homeDir string, castles []castle.Castle) Model {
	roots := make([]string, len(castles))
	for index, castleEntry := range castles {
		roots[index] = castleEntry.Root
	}

	model := Model{homeDir: homeDir, castles: castles, roots: roots}

	if len(castles) == 1 {
		model.selectCastle(0)
	} else {
		model.screen = screenPicker
	}

	return model
}

func (model *Model) selectCastle(index int) {
	// Ensures the UI shows the loading state on first run, otherwise we'd get a blank dashboard.
	model.loading = true

	model.castle = model.castles[index]
	model.repo = git.New(model.castle.Root)
	model.subdirs, _ = castle.Subdirs(model.castle.Root)
	model.linker = &link.Linker{
		Castle:         model.castle,
		HomeDir:        model.homeDir,
		Subdirs:        model.subdirs,
		AllCastleRoots: model.roots,
	}
	model.screen = screenDashboard
}

// Bubble Tea's startup hook - we need to handle that the repo may not exist yet until
// the first castle has been selected.
func (model Model) Init() tea.Cmd {
	if model.repo == nil {
		return nil
	}
	// on startup we need to trigger a reload, so we get the reload command to run,
	// and ignore the returned model, since Init only cares about the command to run.
	_, cmd := model.reload()
	return cmd
}

// When Bubble Tea calls `Update`, it will be given a `loadedMsg` with a fresh snapshot
// of the castle, which is embedded within the root Model. This means the various
// bits of the UI can read the data they need, without it being copied across after
// every update, and without blocking the render loop while the snapshot is being built.
type snapshot struct {
	branch      string
	remote      string
	ahead       int
	behind      int
	upstreamErr error
	files       []git.FileStatus
	actions     []link.Action
	groups      fileGroups
	health      linkSummary
}

// loadedMsg fires when the new snapshot has been built.
type loadedMsg struct {
	snapshot
	err error
}

// The reload Bubble Tea commands sets up the job for reloading the snapshot, and how to return
// it via the loadedMsg, so the Update loop can embed it into the root Model via the snapshot.
func (model Model) reload() (Model, tea.Cmd) {
	model.loading = true

	repo, linker := model.repo, model.linker
	return model, func() tea.Msg {
		var msg loadedMsg
		var err error

		if msg.branch, err = repo.Branch(); err != nil {
			msg.err = err
			return msg
		}

		msg.remote, _ = repo.RemoteURL()

		msg.ahead, msg.behind, msg.upstreamErr = repo.AheadAndBehind("origin/" + msg.branch)

		if msg.files, err = repo.Status(); err != nil {
			msg.err = err
			return msg
		}

		if msg.actions, err = linker.Plan(); err != nil {
			msg.err = err
			return msg
		}

		msg.groups = groupFiles(msg.files)
		msg.health = summariseLinks(msg.actions)

		return msg
	}
}

// execDoneMsg is sent when the execGit command has finished.
type execDoneMsg struct {
	label string
	err   error
}

// execGit hands the terminal to `git`, so it can work normally with whatever config the user has
// for things like commit signing, and SSH prompts, etc. `git [commit|push|fetch|pull]` all use this.
func (model Model) execGit(label string, args ...string) tea.Cmd {
	return tea.ExecProcess(model.repo.Command(args...), func(err error) tea.Msg {
		return execDoneMsg{label: label, err: err}
	})
}

// Update is called by Bubble Tea once per message, in arrival order.
// It handles the message, updates the Model, and optionally returns another Cmd to run.
// The commands run on their own goroutines, and send an update message when done, which
// end up here with the rest to be processed. This ensures slow-running jobs never block
// the main loop, so the UI remains responsive.
func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// A pull runs in multiple stages, each one is handled by the pull.go state machine,
	// keeping all the pull logic together.
	case pullMarkedMsg, pullIncomingMsg, pullReportMsg, pullRelinkedMsg:
		return model.updatePullMsg(msg)

	case tea.WindowSizeMsg:
		// Used to adjust the window sizes, for example the split when a diff is being shown,
		// or the help text bar adjusting to a new window size, etc.
		model.width, model.height = msg.Width, msg.Height
		return model, nil

	case loadedMsg:
		// A loadedMsg fires when the castle reload is completed.
		model.loading = false

		// If we got errors updating, capture those, but leave the previous snapshot in place
		// so the user can still see the last "known good" state of the castle.
		if msg.err != nil {
			model.err = msg.err
			return model, nil
		}

		model.err = nil
		// The snapshot is an embedded field on the main model, so the various other bits of the UI
		// can just read attributes like `model.branch` and `model.files` directly.
		model.snapshot = msg.snapshot

		// Here we sync the Push and Relink screens with the new snapshots, this allows us to
		// preserve the current cursor position, and any selections the user already made,
		// while bringing in any new files that appeared, or removing any that no longer exist,
		// for a much better user experience than just wiping the screen and starting over.
		model.push.reconcile(model.groups)
		model.relink.reconcile(model.actions)

		// Make sure we refresh the diff if the "push" screen is open, so it always
		// matches whatever file the cursor is on.
		if model.screen == screenPush {
			return model, model.loadDiff()
		}

		return model, nil

	case diffMsg:
		// Updates the current diff in the side panel, or shows "(no textual change)"
		// if the file is binary or has no textual diff.
		if msg.err != nil {
			model.push.diff = "could not read diff: " + msg.err.Error()
			return model, nil
		}

		if msg.body == "" {
			model.push.diff = "(no textual change)"
			return model, nil
		}

		model.push.diff = msg.body

		return model, nil

	case execDoneMsg:
		// The execDoneMsg is sent when an exec command is done.
		return model.handleExecDone(msg)

	case tea.KeyPressMsg:
		// Bubble Tea sends keypresses to the Update loop, so we can handle them here.
		return model.handleKey(msg)

	default:
		// Bubble Tea only ever triggers the root Update itself, and all messages are on one flat queue,
		// so we need to pass any unhandled messages along to child components that might be expecting
		// them. Currently that is only the "push" screen, which needs to see `BlinkMsg` messages
		// to ensure the cursor blinks on and off as expected.
		// If we add more child components that might need to see messages, we'll need to
		// add them here, or look into `tea.Batch`.
		var cmd tea.Cmd
		model.push.message, cmd = model.push.message.Update(msg)
		return model, cmd
	}
}

// handleKey handles keypresses for the current screen.
func (model Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// We need to be able to always quit, no matter where we are in the app.
	if key == "ctrl+c" {
		return model, tea.Quit
	}

	// Each screen has its own key handling logic, so we pass the keypress along
	// to the appropriate handler for the current screen.
	switch model.screen {
	case screenPicker:
		return model.handlePickerKey(key)
	case screenDashboard:
		return model.handleDashboardKey(key)
	case screenPush:
		return model.handlePushKey(msg, key)
	case screenPull:
		return model.handlePullKey(key)
	case screenRelink:
		return model.handleRelinkKey(key)
	}

	return model, nil
}

// This fires when the execGit command has finished, and we need to route the result
// to the correct place depending on if we're pushing or pulling.
func (model Model) handleExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case screenPush:
		return model.pushExecDone(msg)
	case screenPull:
		return model.pullExecDone(msg)
	}

	return model.reload()
}

// keys for the castle picker screen.
func (model Model) handlePickerKey(key string) (tea.Model, tea.Cmd) {
	switch key {

	case "q", "esc":
		return model, tea.Quit

	case "up", "k":
		if model.pickerCursor > 0 {
			model.pickerCursor--
		}

	case "down", "j":
		if model.pickerCursor < len(model.castles)-1 {
			model.pickerCursor++
		}

	case "enter":
		model.selectCastle(model.pickerCursor)
		return model.reload()

	}

	return model, nil
}

// keys for the main status dashboard. Most users of homesync will have a single castle
// so they'll end up here most of the time, bypassing the castle picker.
func (model Model) handleDashboardKey(key string) (tea.Model, tea.Cmd) {
	switch key {

	case "q", "esc":
		return model, tea.Quit

	case "r":
		model.notice = ""
		return model.reload()

	case "p":
		model.screen = screenPush
		model.notice = ""
		model.push.reconcile(model.groups)
		// triggers the diff load right away so the user sees the diff for the first file
		// as soon as the screen loads, rather than having to move the cursor first.
		return model, model.loadDiff()

	case "u":
		model.screen = screenPull
		model.notice = ""
		model.pull = pullState{}
		return model, nil

	case "l":
		model.screen = screenRelink
		model.notice = ""
		model.relink.reconcile(model.actions)
		return model, nil
	}

	return model, nil
}

// The main Bubble Tea render loop.
func (model Model) View() tea.View {
	var view tea.View

	// We use the alt screen to avoid cluttering the user's scrollback with the UI,
	// and making it a mess when they exit the app.
	view.AltScreen = true
	view.WindowTitle = "homesync"

	body := ""
	switch model.screen {

	case screenPicker:
		body = model.viewPicker()

	case screenDashboard:
		body = model.viewDashboard()

	case screenPush:
		body = model.viewPush()

	case screenPull:
		body = model.viewPull()

	case screenRelink:
		body = model.viewRelink()

	}

	view.Content = model.chrome(body)

	return view
}

// contentWidth is the width a screen may draw into, inside the titlebar and footer padeding.
func (model Model) contentWidth() int {
	if model.width < 1 {
		return fallbackWidth - 2*chromePadX
	}
	return model.width - 2*chromePadX
}

// help text renders at the bottom of the screen, but needs to be clipped to the
// current width, to prevent overflow/wrapping.
func (model Model) help(pairs ...[2]string) string {
	return helpWidth(model.contentWidth(), pairs...)
}

// chrome draws the header/footer, and and any pending notices/errors around a screen.
func (model Model) chrome(body string) string {
	name := model.castle.Name
	if name == "" {
		name = "no castle selected"
	}

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		titleStyle.Render("homesync"),
		subtleStyle.Render("  "+name),
	)

	parts := []string{header, "", body}

	if model.notice != "" {
		parts = append(parts, "", okStyle.Render(trimRight(model.notice, model.contentWidth())))
	}

	if model.err != nil {
		parts = append(parts, "", errStyle.Render(trimRight("Error: "+model.err.Error(), model.contentWidth())))
	}

	framed := lipgloss.NewStyle().
		Padding(chromePadY, chromePadX).
		Render(lipgloss.JoinVertical(lipgloss.Left, parts...))

	// Ensure the frame is clipped to the current window size, to prevent overflow/wrapping.
	return clipFrame(framed, model.width, model.height)
}

// viewPicker allows a user to pick a castle if they have more than one.
// If they only have a single castle, it is automatically selected, and
// the screen is skipped.
func (model Model) viewPicker() string {
	lines := []string{headingStyle.Render("Select a castle"), ""}

	for index, castleEntry := range model.castles {
		marker := "  "
		style := valueStyle
		if index == model.pickerCursor {
			marker = "> "
			style = selectedStyle
		}
		lines = append(lines, marker+style.Render(castleEntry.Name)+subtleStyle.Render("  "+castleEntry.Root))
	}

	lines = append(lines, "", model.help([2]string{"↑/↓", "move"}, [2]string{"enter", "select"}, [2]string{"q", "quit"}))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
