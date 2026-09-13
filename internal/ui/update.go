package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Rylon/homesync/internal/update"
)

// Homesync checks for new versions on launch, and the user can choose to apply them from the Update screen.

type updateState struct {
	checked   bool
	available bool
	release   update.Release
	applying  bool
	applied   bool
	err       error
}

type updateCheckedMsg struct {
	release update.Release
	found   bool
	err     error
}

type updateAppliedMsg struct {
	err error
}

const updateCheckTimeout = 5 * time.Second
const updateApplyTimeout = 2 * time.Minute

// checkForUpdate reads GitHub releases to see if an update is available.
func (model Model) checkForUpdate() tea.Cmd {
	if update.IsDevMode(model.checker.Version) {
		return nil
	}

	checker := model.checker
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()

		release, found, err := checker.Latest(ctx)
		return updateCheckedMsg{release: release, found: found, err: err}
	}
}

// applyUpdate downloads the release, and applies it (if the checksums match).
func (model Model) applyUpdate() (tea.Model, tea.Cmd) {
	if !model.update.available || model.update.applying || model.update.applied {
		return model, nil
	}

	model.update.applying = true
	model.update.err = nil

	checker, release := model.checker, model.update.release
	return model, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateApplyTimeout)
		defer cancel()

		return updateAppliedMsg{err: checker.Apply(ctx, release)}
	}
}

// handleUpdateMsg deals with the two update related messages.
func (model Model) handleUpdateMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case updateCheckedMsg:
		model.update.checked = true
		model.update.err = msg.err
		model.update.available = msg.found
		model.update.release = msg.release

	case updateAppliedMsg:
		model.update.applying = false
		model.update.err = msg.err
		model.update.applied = msg.err == nil
	}

	return model, nil
}

// handleUpdateKey handles keypresses for the Update screen.
func (model Model) handleUpdateKey(key string) (tea.Model, tea.Cmd) {
	switch key {

	case "esc", "q":
		model.screen = screenDashboard
		return model, nil

	case "enter":
		return model.applyUpdate()
	}

	return model, nil
}

// versionText is the Version row on the main dashboard.
func (model Model) versionText() string {
	currentVersion := update.Label(model.checker.Version)

	if update.IsDevMode(model.checker.Version) {
		return valueStyle.Render(currentVersion + separator + subtleStyle.Render("local build"))
	}

	state := model.update
	latestVersion := update.Label(state.release.Version)

	switch {
	case state.applied:
		return currentVersion + separator + okStyle.Render(latestVersion+" installed!")

	case state.applying:
		return currentVersion + separator + warnStyle.Render("downloading "+latestVersion+"...")

	// This can only happen if the check succeeded, but a later error occurred when applying,
	// for example a failed download.
	case state.err != nil && state.available:
		return currentVersion + separator + errStyle.Render("update to "+latestVersion+" failed: "+state.err.Error())

	case state.err != nil:
		return currentVersion + separator + subtleStyle.Render("could not check for updates")

	case state.available:
		return currentVersion + separator + warnStyle.Render(latestVersion+" available 🎉, press U to update")

	case state.checked:
		return currentVersion + separator + subtleStyle.Render("latest")
	}

	return currentVersion
}

// viewUpdate shows the details for the new update, including version number and the changelog
// from the GitHub Release, so the user can see what's new before installing.
func (model Model) viewUpdate() string {
	state := model.update
	width := model.contentWidth()
	latestVersion := update.Label(state.release.Version)

	versionLine := valueStyle.Render(update.Label(model.checker.Version)) + subtleStyle.Render("  →  ") + warnStyle.Render(latestVersion)

	if !state.release.PublishedAt.IsZero() {
		versionLine += subtleStyle.Render("  published " + state.release.PublishedAt.Format("2 Jan 2006"))
	}

	header := []string{
		headingStyle.Render("Update is available!"),
		"",
		trimRight(versionLine, width),
		"",
	}

	footer := []string{"", subtleStyle.Render(trimRight(state.release.URL, width)), ""}

	switch {
	case state.applied:
		footer = append(footer,
			okStyle.Render(trimRight(latestVersion+" installed, restart to apply changes.", width)),
			"",
			model.help([2]string{"esc", "back"}),
		)

	case state.applying:
		footer = append(footer,
			warnStyle.Render(trimRight("downloading "+latestVersion+"...", width)),
			"",
			model.help([2]string{"esc", "back"}),
		)

	case state.err != nil:
		footer = append(footer,
			errStyle.Render(trimRight("update failed: "+state.err.Error(), width)),
			"",
			model.help([2]string{"enter", "try again"}, [2]string{"esc", "back"}),
		)

	default:
		footer = append(footer, model.help([2]string{"enter", "download and install"}, [2]string{"esc", "back"}))
	}

	// The notes get whatever terminal height is left when we subtract the fixed header and footer rows.
	reserved := chromeOverhead + len(header) + lipgloss.Height(strings.Join(footer, "\n"))

	// Notices and errors can each take up two lines if present, so include them too.
	if model.notice != "" {
		reserved += 2
	}
	if model.err != nil {
		reserved += 2
	}

	lines := append(header, renderReleaseNotes(state.release.Notes, width, model.height-reserved)...)
	lines = append(lines, footer...)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderReleaseNotes formats the release notes from the body of the GitHub Release.
// GoReleaser writes these as Markdown, with a `## Changelog` heading, and a list of bullet points
// for each commit, so we do some basic conversion here.
func renderReleaseNotes(notes string, width, height int) []string {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return []string{subtleStyle.Render("No release notes were published.")}
	}

	lines := truncateLines(strings.Split(notes, "\n"), height)

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "#"):
			out = append(out, headingStyle.Render(trimRight(strings.TrimLeft(line, "# "), width)))

		case strings.HasPrefix(line, "* "), strings.HasPrefix(line, "- "):
			out = append(out, trimRight(subtleStyle.Render("  • ")+valueStyle.Render(line[2:]), width))

		default:
			out = append(out, valueStyle.Render(trimRight(line, width)))
		}
	}

	return out
}
