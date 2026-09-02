// Package ui is the Bubble Tea interface. It calls the various internal packages
// to figure out the state of the castle, the home dir, and what actions are needed,
// and deals with rendering the UI for all of that to the screen.
package ui

import (
	tea "charm.land/bubbletea/v2"

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
	return model.reload()
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
func (model Model) reload() tea.Cmd {
	repo, linker := model.repo, model.linker
	return func() tea.Msg {
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
		model.push.resize(model.width, model.height)
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
		model.push.sync(model.groups)
		model.relink.sync(model.actions)

		// Make sure we refresh the diff if the "push" screen is open, so it always
		// matches whatever file the cursor is on.
		if model.screen == screenPush {
			return model, model.diffCmd()
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
