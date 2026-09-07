package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Rylon/homesync/internal/git"
	"github.com/Rylon/homesync/internal/link"
)

// pullStep tracks where the user is in the pull.
type pullStep int

const (
	pullGate pullStep = iota
	pullFetching
	pullPreview
	pullIntegrating
	pullConflicted
	pullReport
)

type pullState struct {
	step pullStep

	before   string       // HEAD before the pull, so the report can show what changed
	incoming []git.Commit // on origin but not yet on HEAD

	conflicts []string
	rebasing  bool

	linkResult link.Result
	diffStat   string
}

// pullIncomingMsg carries what the fetch found waiting on origin, and the `before` state.
type pullIncomingMsg struct {
	before  string
	commits []git.Commit
	err     error
}

// pullReportMsg carries the outcome of the pull and the `linkResult` from the linking
// which runs after a successful pull.
type pullReportMsg struct {
	conflicts  []string
	rebasing   bool
	diffStat   string
	linkResult link.Result
	err        error
}

// Handles all keypresses for the pull screen.
func (model Model) handlePullKey(key string) (tea.Model, tea.Cmd) {
	switch model.pull.step {

	case pullGate:
		return model.handlePullGateKey(key)

	case pullPreview:
		return model.handlePullPreviewKey(key)

	case pullConflicted:
		if key == "esc" || key == "q" {
			model.screen = screenDashboard
			return model, nil
		}

		if key == "a" {
			return model, model.execGit("abort", model.abortArgs()...)
		}

		return model, nil

	case pullReport:
		if key == "esc" || key == "q" || key == "enter" {
			model.screen = screenDashboard
			model.notice = ""
			return model, nil
		}

		return model, nil
	}

	// for all other modes, we only allow going back to the main dashboard
	if key == "esc" || key == "q" {
		model.screen = screenDashboard
	}

	return model, nil
}

func (model Model) handlePullGateKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "q" {
		model.screen = screenDashboard
		return model, nil
	}

	// pulls are gated by uncommitted changes, we avoid dealing with stashing as it's too easy
	// to lose changes with a bad stash pop, so we direct the user to the push screen to deal with
	// their local changes first.
	if gateForPull(model.files).Blocked {
		if key == "p" {
			model.screen = screenPush
			model.push.reconcile(model.groups)
		}
		return model, nil
	}

	if key == "enter" {
		model.pull.step = pullFetching
		return model, model.execGit("fetch", "fetch", "origin")
	}

	return model, nil
}

// fetchIncoming runs after the fetch, then builds the lists of commits that have not been applied yet.
// We can't rely on the fetch result only for this, as it may be empty (a second fetch following a
// previously successful one wouldn't show any new commits), so we build it by comparing
// HEAD to `origin/<branch>` directly.
func (model Model) fetchIncoming() tea.Cmd {
	repo, branch := model.repo, model.branch
	return func() tea.Msg {
		before, err := repo.RevParse("HEAD")
		if err != nil {
			return pullIncomingMsg{err: err}
		}

		commits, err := repo.LogRange("HEAD", "origin/"+branch)
		return pullIncomingMsg{before: before, commits: commits, err: err}
	}
}

func (model Model) handlePullPreviewKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		model.screen = screenDashboard
		return model, nil

	case "enter":
		if len(model.pull.incoming) == 0 {
			model.screen = screenDashboard
			model.notice = "already up to date, nothing to integrate"
			return model, nil
		}
		model.pull.step = pullIntegrating

		// Using `git pull` here allows the user's own `pull.rebase` config to be honoured (if present).
		return model, model.execGit("pull", "pull")
	}

	return model, nil
}

func (model Model) abortArgs() []string {
	if model.pull.rebasing {
		return []string{"rebase", "--abort"}
	}
	return []string{"merge", "--abort"}
}

func (model Model) pullExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	switch msg.label {
	case "abort":
		if msg.err != nil {
			model.err = fmt.Errorf("abort failed: %w", msg.err)
			return model, nil
		}
		model.screen = screenDashboard
		model.notice = "aborted, the castle is back where it started"
		return model.reload()

	case "fetch":
		if msg.err != nil {
			model.err = fmt.Errorf("fetch failed: %w", msg.err)
			model.pull.step = pullGate
			return model, nil
		}
		model.err = nil
		return model, model.fetchIncoming()

	case "pull":
		// A failed pull usually means conflicts, so inspect rather than trust
		// the exit code alone.
		return model, model.inspectAfterPull(msg.err)
	}
	return model.reload()
}

// inspectAfterPull works out if the pull was completed cleanly, and if so, tries to link automatically
func (model Model) inspectAfterPull(pullErr error) tea.Cmd {
	repo, linker := model.repo, model.linker
	before := model.pull.before

	return func() tea.Msg {
		conflicts, err := repo.ConflictedPaths()
		if err != nil {
			return pullReportMsg{err: err}
		}

		rebasing := repo.IsRebasing()

		if len(conflicts) > 0 || rebasing {
			return pullReportMsg{conflicts: conflicts, rebasing: rebasing}
		}

		if pullErr != nil {
			return pullReportMsg{err: pullErr}
		}

		// DiffStats handles if a rebase was performed, so existing commits with new hashes after a rebase
		// won't be reported as new commits.
		report := pullReportMsg{}
		report.diffStat, _ = repo.DiffStat(before, "HEAD")

		plan, err := linker.Plan()
		if err != nil {
			report.err = fmt.Errorf("pulled, but could not plan the symlinks: %w", err)
			return report
		}

		report.linkResult, err = linker.Apply(plan)
		if err != nil {
			report.err = fmt.Errorf("pulled, but linking failed: %w", err)
		}

		return report
	}
}

// updatePullMsg handles the pull-specific messages from the root Update.
func (model Model) updatePullMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pullIncomingMsg:
		if msg.err != nil {
			model.err = msg.err
			model.pull.step = pullGate
			return model, nil
		}

		model.pull.before, model.pull.incoming = msg.before, msg.commits
		model.pull.step = pullPreview

		return model, nil

	case pullReportMsg:
		if len(msg.conflicts) > 0 || msg.rebasing {
			model.pull.conflicts, model.pull.rebasing = msg.conflicts, msg.rebasing
			model.pull.step = pullConflicted
			return model, nil
		}

		model.err = msg.err
		model.pull.diffStat, model.pull.linkResult = msg.diffStat, msg.linkResult
		model.pull.step = pullReport

		return model.reload()
	}

	return model, nil
}

// There are different modes depending on which stage of the pull the user is in.
func (model Model) viewPull() string {
	switch model.pull.step {

	case pullGate:
		return model.viewPullGate()

	case pullFetching:
		return subtleStyle.Render("fetching from origin...")

	case pullPreview:
		return model.viewPullPreview()

	case pullIntegrating:
		return subtleStyle.Render("integrating and symlinking...")

	case pullConflicted:
		return model.viewPullConflicted()

	case pullReport:
		return model.viewPullReport()
	}

	return ""
}

// The gate is open if there are no tracked files with uncommitted changes present locally.
// The user is required to commit any local changes before they are able to pull, this is to
// prevent the need to deal with stashing, where a failed stash pop could accidentally
// wipe out the user's local changes.
func (model Model) viewPullGate() string {
	gate := gateForPull(model.files)

	// If there are no local changes, we should be safe to proceed.
	if !gate.Blocked {
		lines := []string{
			headingStyle.Render("Pull from origin"),
			"",
			valueStyle.Render(trimRight("There are no local changes to tracked files, so a pull is safe.", model.contentWidth())),
		}

		if len(model.groups.New) > 0 {
			lines = append(lines, subtleStyle.Render(
				fmt.Sprintf("%d untracked file(s).", len(model.groups.New))))
		}

		lines = append(lines, "", model.help([2]string{"enter", "fetch"}, [2]string{"esc", "back"}))

		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	// If there are uncommitted changes to tracked files, we list them here, and ask the user to deal with it.
	lines := []string{
		warnStyle.Render(trimRight("Commit or discard these tracked changes before pulling:", model.contentWidth())),
		subtleStyle.Render(trimRight("homesync never stashes, in order to avoid potential data loss through a failed stash pop.", model.contentWidth())),
		"",
	}

	// list of tracked files with uncommitted changes
	for _, path := range gate.Blockers {
		lines = append(lines, warnStyle.Render(trimRight("  "+path, model.contentWidth())))
	}

	lines = append(lines, "", model.help([2]string{"p", "go to push"}, [2]string{"esc", "back"}))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (model Model) viewPullPreview() string {
	if len(model.pull.incoming) == 0 {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			okStyle.Render(trimRight("Already up to date.", model.contentWidth())),
			"",

			model.help([2]string{"enter", "back"}, [2]string{"esc", "back"}),
		)
	}

	lines := []string{
		headingStyle.Render(fmt.Sprintf("new %s on origin", plural(len(model.pull.incoming), "commit"))),
		subtleStyle.Render(trimRight("Nothing has been pulled yet.", model.contentWidth())),
		"",
	}

	// list of incoming commits
	for _, commit := range model.pull.incoming {
		lines = append(lines, trimRight(subtleStyle.Render("  "+commit.Hash+"  ")+valueStyle.Render(commit.Subject), model.contentWidth()))
	}

	lines = append(lines, "", model.help([2]string{"enter", "pull and symlink"}, [2]string{"esc", "cancel"}))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (model Model) viewPullConflicted() string {
	actionText := "merge"
	if model.pull.rebasing {
		actionText = "rebase"
	}
	heading := fmt.Sprintf("The %s failed due to conflicts.", actionText)

	lines := []string{
		errStyle.Render(trimRight(heading, model.contentWidth())),
		warnStyle.Render(trimRight("These files contain conflict markers:", model.contentWidth())),
		subtleStyle.Render(trimRight("Depending on the file, the conflict markers may break the syntax of the file.", model.contentWidth())),
		"",
	}

	// list of files with conflicts
	for _, path := range model.pull.conflicts {
		lines = append(lines, errStyle.Render(trimRight("  "+path, model.contentWidth())))
	}

	// TODO: we should probably offer a "resolve in editor" option here, but for now we just tell the user to do it themselves.

	finishCommand := "git add <file> && git commit"
	if model.pull.rebasing {
		finishCommand = "git add <file> && git rebase --continue"
	}

	lines = append(lines,
		"",
		subtleStyle.Render(trimRight("Outside homesync, resolve these conflicts in your editor, then run:", model.contentWidth())),
		valueStyle.Render(trimRight("  "+finishCommand, model.contentWidth())),
		"",
		model.help([2]string{"a", "abort and put the castle back how it was"}, [2]string{"esc", "keep the conflicts for manual resolution"}),
	)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (model Model) viewPullReport() string {
	// The chrome already prints `model.err`, so we just need to add a suitable heading and footer here.
	if model.err != nil {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			headingStyle.Render("Pull did not complete"),
			"",
			model.help([2]string{"enter", "back to dashboard"}),
		)
	}

	lines := []string{headingStyle.Render("Pull complete"), ""}

	// An empty preview never reaches the report, so there is always at least one commit to list.
	lines = append(lines, valueStyle.Render("Arrived from origin:"))
	for _, commit := range model.pull.incoming {
		lines = append(lines, trimRight(subtleStyle.Render("  "+commit.Hash+"  ")+valueStyle.Render(commit.Subject), model.contentWidth()))
	}

	if model.pull.linkResult.Created > 0 {
		lines = append(lines, "", okStyle.Render(fmt.Sprintf("Created %s.", plural(model.pull.linkResult.Created, "symlink"))))
	}

	// Wait until loading has finished before showing any problems, so we know the state is up to date.
	if !model.loading && model.health.Problems() > 0 {
		lines = append(lines, warnStyle.Render(
			fmt.Sprintf("%s need attention. Press l on the main dashboard.", plural(model.health.Problems(), "symlink"))))
	}

	if model.pull.diffStat != "" {
		lines = append(lines, "", valueStyle.Render("Changed files:"))
		for _, line := range strings.Split(strings.TrimRight(model.pull.diffStat, "\n"), "\n") {
			lines = append(lines, subtleStyle.Render(trimRight(line, model.contentWidth())))
		}
		lines = append(lines, warnStyle.Render(trimRight("Apps that use these files may need to be restarted to see the changes.", model.contentWidth())))
	}

	lines = append(lines, "", model.help([2]string{"enter", "back to dashboard"}))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
