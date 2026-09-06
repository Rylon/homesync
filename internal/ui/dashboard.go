package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// The dashboard is the main screen, showing an overview of the castle, whether it is ahead or behind
// the Git repo, and whether the symlinks are all in place.

func (model Model) viewDashboard() string {
	if model.loading {
		return subtleStyle.Render("reading castle…")
	}

	rows := []string{
		rowFit("Castle", valueStyle.Render(model.tildePath(model.castle.Root)), model.contentWidth()),
		rowFit("Origin", valueStyle.Render(model.originText()), model.contentWidth()),
		rowFit("Branch", model.branchText(), model.contentWidth()),
		"",
		rowFit("Changes", model.changesText(), model.contentWidth()),
		rowFit("Links", model.linksText(), model.contentWidth()),
		"",
		menuItem("p", "push", "review changes, commit and publish"),
		menuItem("u", "pull", "fetch, integrate and relink"),
		menuItem("l", "links", "create symlinks from castle to $HOME"),
		"",
		model.help([2]string{"r", "refresh"}, [2]string{"q", "quit"}),
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// rowFit renders a label and its value. We trim the values to ensure the row stays within
// the desired content `width`.
func rowFit(label, value string, width int) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		labelStyle.Render(label),
		trimRight(value, width-labelWidth),
	)
}

// menu items are shown in three columns, with a `key`, a `name`, and a `description`, for example:
// "p   push      review changes, commit and publish"
func menuItem(key, name, description string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(4).Render(keyStyle.Render(key)),
		lipgloss.NewStyle().Width(10).Render(valueStyle.Render(name)),
		subtleStyle.Render(description),
	)
}

func (model Model) originText() string {
	if model.remote == "" {
		return errStyle.Render("no origin configured")
	}
	return model.remote
}

// branchText shows the current branch, and whether it is ahead or behind the remote, and if so,
// by how many commits.
func (model Model) branchText() string {

	// Shouldn't really happy with a normal castle, but we guard against it.
	if model.upstreamErr != nil {
		return valueStyle.Render(model.branch) + separator +
			warnStyle.Render("no origin/"+model.branch+" ref yet, pull to fetch it")
	}

	var state string
	switch {

	// up to date!
	case model.ahead == 0 && model.behind == 0:
		state = okStyle.Render("up to date with origin/" + model.branch)

	// ahead and behind, so we have diverged.
	case model.ahead > 0 && model.behind > 0:
		state = warnStyle.Render(fmt.Sprintf("%s ahead, %s behind", plural(model.ahead, "commit"), plural(model.behind, "commit")))

	// only ahead, so we have commits to push.
	case model.ahead > 0:
		state = warnStyle.Render(plural(model.ahead, "commit") + " to push")

	// only behind, so we have commits to pull.
	default:
		state = warnStyle.Render(plural(model.behind, "commit") + " to pull")
	}

	return valueStyle.Render(model.branch) + separator + state +
		subtleStyle.Render("  (as of last refresh)")
}

// changesText shows how many files have changed, and how many untracked files there are.
func (model Model) changesText() string {
	changed, added := len(model.groups.Changed), len(model.groups.New)
	if changed == 0 && added == 0 {
		return okStyle.Render("nothing to commit")
	}

	var parts []string

	if changed > 0 {
		parts = append(parts, warnStyle.Render(plural(changed, "change")))
	}

	if added > 0 {
		parts = append(parts, newStyle.Render(fmt.Sprintf("%d untracked", added)))
	}

	return strings.Join(parts, separator)
}

// linksText shows the status of the symlinks, how many are in place, and if there are
// any problems that need fixing.
func (model Model) linksText() string {
	linked := okStyle.Render(fmt.Sprintf("%d linked", model.health.Identical))

	if model.health.Problems() == 0 {
		return linked + separator + subtleStyle.Render("nothing to repair")
	}

	parts := []string{linked}

	if model.health.Missing > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("%d missing", model.health.Missing)))
	}
	if model.health.Conflicts > 0 {
		parts = append(parts, errStyle.Render(fmt.Sprintf("%d conflicting", model.health.Conflicts)))
	}
	if model.health.Refused > 0 {
		parts = append(parts, errStyle.Render(fmt.Sprintf("%d refused", model.health.Refused)))
	}

	return strings.Join(parts, separator)
}
