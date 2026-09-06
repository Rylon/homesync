// Appearance only: colours, styles, and dimensions only.

package ui

import (
	"charm.land/lipgloss/v2"
)

// chromePadding is the horizontal and vertical padding that chrome adds.
const (
	chromePadX = 2
	chromePadY = 1
	// chromeOverhead counts the lines `chrome` spends on padding, the header and
	// the blank line under it.
	chromeOverhead = 2*chromePadY + 2
	// fallbackWidth applies before the first WindowSizeMsg arrives.
	fallbackWidth = 80
	// labelWidth is the fixed column for dashboard row labels.
	labelWidth = 10
)

var (
	colAccent = lipgloss.Color("#8caaee")
	colMuted  = lipgloss.Color("#838ba7")
	colOK     = lipgloss.Color("#a6d189")
	colWarn   = lipgloss.Color("#e5c890")
	colErr    = lipgloss.Color("#e78284")
	colNew    = lipgloss.Color("#ca9ee6")
	colFg     = lipgloss.Color("#c6d0f5")
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	subtleStyle   = lipgloss.NewStyle().Foreground(colMuted)
	labelStyle    = lipgloss.NewStyle().Foreground(colMuted).Width(labelWidth)
	valueStyle    = lipgloss.NewStyle().Foreground(colFg)
	okStyle       = lipgloss.NewStyle().Foreground(colOK)
	warnStyle     = lipgloss.NewStyle().Foreground(colWarn)
	errStyle      = lipgloss.NewStyle().Foreground(colErr)
	newStyle      = lipgloss.NewStyle().Foreground(colNew)
	keyStyle      = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colFg)
	headingStyle  = lipgloss.NewStyle().Bold(true).Foreground(colMuted)
	dividerStyle  = lipgloss.NewStyle().Foreground(colMuted)
)

var (
	diffAddStyle    = lipgloss.NewStyle().Foreground(colOK)
	diffDelStyle    = lipgloss.NewStyle().Foreground(colErr)
	diffHunkStyle   = lipgloss.NewStyle().Foreground(colAccent)
	diffHeaderStyle = lipgloss.NewStyle().Foreground(colMuted)
)

// Used to visually separate related bits of text on a row, making sure they stay visually aligned.
var separator = subtleStyle.Render("  ·  ")

// Divides the two panels on the push screen.
var divider = dividerStyle.Render(" │ ")
