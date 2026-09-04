package ui

import (
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// helpWidth renders key hints, handling line breaks as needed, to stay within the width limit.
//
// `lipgloss.JoinVertical` pads every line to the widest one, so an overly long help text row
// stretches the whole frame, and ends up wrapping/scrolling on the alt-screen in some terminals
// (eg iTerm).
func helpWidth(width int, pairs ...[2]string) string {
	if width < 1 {
		width = fallbackWidth
	}

	separatorWidth := lipgloss.Width(separator)

	var lines []string
	current := ""
	currentWidth := 0

	for _, pair := range pairs {
		item := keyStyle.Render(pair[0]) + " " + subtleStyle.Render(pair[1])
		itemWidth := lipgloss.Width(item)

		switch {
		case current == "":
			current, currentWidth = item, itemWidth
		case currentWidth+separatorWidth+itemWidth <= width:
			current += separator + item
			currentWidth += separatorWidth + itemWidth
		default:
			lines = append(lines, current)
			current, currentWidth = item, itemWidth
		}
	}

	if current != "" {
		lines = append(lines, current)
	}

	return strings.Join(lines, "\n")
}

// clipFrame stops the frame from exceeding the terminal size, preventing any overflow
// from wrapping/scrolling on the alt-screen, leaking the UI into shell scrollback
// on terminals that support that (eg iTerm).
func clipFrame(content string, width, height int) string {
	lines := strings.Split(content, "\n")

	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}

	if width > 0 {
		for index, line := range lines {
			if lipgloss.Width(line) > width {
				lines[index] = ansi.Truncate(line, width, "")
			}
		}
	}

	return strings.Join(lines, "\n")
}

// renderDiff adds colours to a diff and ensures it is clipped within the side panel.
func renderDiff(body string, width, height int) string {
	if height < 1 {
		height = 1
	}

	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) > height {
		// Spend one of the allotted rows on the marker, so the block is exactly
		// `height` lines. Returning `height+1` would push the help row out of frame.
		lines = append(lines[:height-1], subtleStyle.Render("… truncated to fit"))
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, styleDiffLine(line, width))
	}

	return strings.Join(out, "\n")
}

// Sets the correct colour to use for each line of the diff, depending on what changed.
func styleDiffLine(line string, width int) string {
	clipped := trimRight(line, width)

	switch {
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"),
		strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "):
		return diffHeaderStyle.Render(clipped)
	case strings.HasPrefix(line, "@@"):
		return diffHunkStyle.Render(clipped)
	case strings.HasPrefix(line, "+"):
		return diffAddStyle.Render(clipped)
	case strings.HasPrefix(line, "-"):
		return diffDelStyle.Render(clipped)
	}
	return valueStyle.Render(clipped)
}

// trimRight clips the end of a string to a given width, adding a `...` suffix.
func trimRight(text string, width int) string {
	if width <= 1 || lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	if len(runes) > width-1 {
		runes = runes[:width-1]
	}
	return string(runes) + "…"
}

// trimLeft does the same as trimRight, but clips the start of a string, adding a `...` prefix.
func trimLeft(text string, width int) string {
	if width <= 1 || lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	if len(runes) > width-1 {
		runes = runes[len(runes)-(width-1):]
	}
	return "…" + string(runes)
}

const maxPreviewBytes = 64 * 1024

// readCapped reads files from disk, so they can be rendered as a diff in the side panel,
// but is capped to 64KB so we don't try to read enormous files into memory for no good reason.
func readCapped(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buf := make([]byte, maxPreviewBytes)
	bytesRead, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	text := string(buf[:bytesRead])
	if bytesRead == maxPreviewBytes {
		text += "\n… truncated"
	}

	return text, nil
}
