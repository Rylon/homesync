package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Rylon/homesync/internal/link"
)

// The Links screen is responsible for creating symlinks from the castle to the $HOME directory,
// and handling conflicts if there are any existing files in $HOME that need to be overwritten.

type linksState struct {
	problems  []link.Action // Create, Conflict, SymlinkConflict and Refused
	cursor    int
	confirmed bool // awaiting confirmation to overwrite the highlighted entry
}

// reconcile takes an updated action plan, and ensures the cursor remains within the new list size.
func (state *linksState) reconcile(actions []link.Action) {
	state.problems = link.Problems(actions)
	state.cursor = clampCursor(state.cursor, len(state.problems))
	state.confirmed = false
}

// currentAction returns the highlighted entry.
func (state linksState) currentAction() (link.Action, bool) {
	if len(state.problems) == 0 {
		return link.Action{}, false
	}
	return state.problems[state.cursor], true
}

// handleLinksKey handles keypresses for the Links screen.
func (model Model) handleLinksKey(key string) (tea.Model, tea.Cmd) {
	// we only allow overwrites to proceed if they've been confirmed by the user
	if model.links.confirmed {
		switch key {

		// y to confirm
		case "y":
			return model.overwriteCurrentFile()

		// any other key cancels
		default:
			model.links.confirmed = false
			model.notice = "left the existing file alone"
			return model, nil
		}
	}

	switch key {
	case "esc", "q":
		model.screen = screenDashboard
		model.notice = ""
		return model, nil

	case "up", "k":
		if model.links.cursor > 0 {
			model.links.cursor--
		}
		return model, nil

	case "down", "j":
		if model.links.cursor < len(model.links.problems)-1 {
			model.links.cursor++
		}
		return model, nil

	case "a":
		return model.applyCreates()

	case "o":
		action, ok := model.links.currentAction()
		if !ok {
			return model, nil
		}

		if action.Kind == link.Refused {
			model.notice = "refused, cannot be symlinked: " + action.Reason
			return model, nil
		}

		if action.Kind == link.Create {
			model.notice = "press a to create the missing symlinks"
			return model, nil
		}

		model.links.confirmed = true
		return model, nil

	case "r":
		return model.reload()
	}

	return model, nil
}

// applyCreates makes the missing links, as that's always safe to do.
func (model Model) applyCreates() (tea.Model, tea.Cmd) {
	result, err := model.linker.Apply(model.links.problems)
	if err != nil {
		model.err = err
		return model, nil
	}

	if len(result.Errors) > 0 {
		model.err = result.Errors[0]
	}

	if result.Created == 0 {
		model.notice = "no missing symlinks to create"
	} else {
		model.notice = fmt.Sprintf("created %s", plural(result.Created, "symlink"))
	}

	return model.reload()
}

// overwriteCurrentFile replaces the current file in the $HOME with the desired file from the castle,
// providing the user has confirmed that decision.
func (model Model) overwriteCurrentFile() (tea.Model, tea.Cmd) {
	action, ok := model.links.currentAction()
	model.links.confirmed = false
	if !ok {
		return model, nil
	}

	if err := model.linker.Overwrite(action); err != nil {
		model.err = err
		return model, nil
	}

	model.err = nil
	model.notice = "replaced " + model.tildePath(action.Destination) + " with a symlink"

	return model.reload()
}

// The main view for the Links screen, shows the list of files needing attention, and let's the user
// provide confirmation for any replacements.
func (model Model) viewLinks() string {
	if len(model.links.problems) == 0 {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			okStyle.Render(fmt.Sprintf("All %d symlinks are correct.", model.health.Identical)),
			"",
			model.help([2]string{"r", "refresh"}, [2]string{"esc", "back"}),
		)
	}

	lines := []string{
		headingStyle.Render("Files needing attention"),
		subtleStyle.Render(trimRight("Creating new symlinks is safe, but replacing an", model.contentWidth())),
		subtleStyle.Render(trimRight("existing file needs confirmation, one file at a time.", model.contentWidth())),
		"",
	}
	for index, action := range model.links.problems {
		marker := "  "
		style := valueStyle

		if index == model.links.cursor {
			marker = "> "
			style = selectedStyle
		}

		lines = append(lines, trimRight(marker+kindLabel(action.Kind)+" "+style.Render(action.Rel), model.contentWidth()))

		if index == model.links.cursor {
			lines = append(lines, "      "+subtleStyle.Render(trimRight(model.linkDetail(action), model.contentWidth()-6)))
		}
	}

	lines = append(lines, "")
	if model.links.confirmed {
		action, _ := model.links.currentAction()
		lines = append(lines,
			errStyle.Render(trimRight("Replace "+model.tildePath(action.Destination)+" with a symlink into the castle?", model.contentWidth())),
			subtleStyle.Render(trimRight("The existing file will be deleted. This cannot be undone.", model.contentWidth())),
			"",
			model.help([2]string{"y", "replace it"}, [2]string{"any other key", "cancel"}),
		)

	} else {
		lines = append(lines, model.help(
			[2]string{"↑/↓", "move"},
			[2]string{"a", "create all missing links"},
			[2]string{"o", "replace only this one"},
			[2]string{"r", "refresh"},
			[2]string{"esc", "back"},
		))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// kindLabel adds padding to each label up to the specified width, so they're visually aligned in a column.
func kindLabel(kind link.Kind) string {
	const width = 9

	// Default label, although `identical` is a `Kind`, it doesn't get shown in the problems list,
	// because it means the symlink is already in place and correct.
	label, style := "identical", subtleStyle

	switch kind {
	case link.Create:
		label, style = "missing", warnStyle

	case link.Conflict:
		label, style = "conflict", errStyle

	case link.SymlinkConflict:
		label, style = "elsewhere", errStyle

	case link.Refused:
		label, style = "refused", errStyle
	}

	return style.Render(fmt.Sprintf("%-*s", width, label))
}

// linkDetail creates human-readable details forthe state of each file.
func (model Model) linkDetail(action link.Action) string {
	switch action.Kind {

	case link.Create:
		return "would link " + model.tildePath(action.Destination) + " to " + model.tildePath(action.Source)

	case link.Conflict:
		return "a file already exists at " + model.tildePath(action.Destination)

	case link.SymlinkConflict:
		return "a symlink already exists at " + model.tildePath(action.Destination) + " pointing at the wrong target " + model.tildePath(action.CurrentTarget)

	case link.Refused:
		return action.Reason
	}

	return ""
}
