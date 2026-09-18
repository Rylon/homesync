package ui

import (
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// markdownLinkPattern matches a `[label](url)` link and captures the label and the URL.
var markdownLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)

// hyperlinkOpenPattern matches an OSC8 open sequence, and captures the URL.
var hyperlinkOSC8OpenPattern = regexp.MustCompile("\x1b\\]8;;([^\x07\x1b]+)")

// renderMarkdown formats raw Markdown into Lipgloss styles:
// * links become real OSC8 hyperlinks.
// * headings are styled according to their level.
// * bullets (* or -) become "•", and we properly handle indenting if they wrap lines.
// * wrap the text to fit the available terminal width.
func renderMarkdown(markdown string, width int) []string {
	bullet := subtleStyle.Render("  • ")
	indent := strings.Repeat(" ", lipgloss.Width(bullet))

	var out []string
	for _, line := range strings.Split(markdown, "\n") {
		switch {
		case strings.HasPrefix(line, "#"):
			out = append(out, formatMarkdownLine(strings.TrimLeft(line, "# "), width, "", "", headingStyle)...)

		case strings.HasPrefix(line, "* "), strings.HasPrefix(line, "- "):
			out = append(out, formatMarkdownLine(line[2:], width, bullet, indent, valueStyle)...)

		default:
			out = append(out, formatMarkdownLine(line, width, "", "", valueStyle)...)
		}
	}

	return out
}

// styleMarkdownLinks converts one paragraph of Markdown into a styled string,
// looking for hyperlinks, and turning them into proper OSC8 hyperlinks.
func styleMarkdownLinks(markdown string, style lipgloss.Style) string {
	var out strings.Builder

	plainText := func(segment string) {
		if segment != "" {
			out.WriteString(style.Render(segment))
		}
	}

	// lastLink is a cursor pointing to the end of the last processed link.
	// We walk through the Markdown text, processing each link we find, and
	// creating an OSC8 style link. When we've processed all links, we append
	// any remaining plaintext after the last one.
	lastLink := 0
	for _, match := range markdownLinkPattern.FindAllStringSubmatchIndex(markdown, -1) {
		plainText(markdown[lastLink:match[0]])

		label, url := markdown[match[2]:match[3]], markdown[match[4]:match[5]]

		// We render the link with the label and the URL in brackets, purely
		// for Apple Terminal which doesn't support OSC8 links still.
		// There's no mechanism to detect OSC8 support, so this seems like
		// a reasonable compromise to ensure the links are still visible.
		out.WriteString(linkStyle.Hyperlink(url).Render(label))
		out.WriteString(" ")
		out.WriteString(subtleStyle.Hyperlink(url).Render("(" + url + ")"))

		lastLink = match[1]
	}

	plainText(markdown[lastLink:])

	return out.String()
}

// tagHyperlinksWithLineNumber sets the OSC8 id of every hyperlink that
// ended up on a wrapped line to the line number. This is to work around a
// potential issue in the renderer:
// https://github.com/charmbracelet/ultraviolet/issues/189
// Using the line number as the ID means each part of the link gets a unique ID
// so the renderer sees a new link for each line, and adds the correct sequence,
// otherwise the link isn't properly rendered on wrapped lines.
// Remove once https://github.com/charmbracelet/ultraviolet/issues/189 is fixed.
func tagHyperlinksWithLineNumber(line string, lineNumber int) string {
	return hyperlinkOSC8OpenPattern.ReplaceAllString(line, "\x1b]8;id="+strconv.Itoa(lineNumber)+";$1")
}

// formatMarkdownLine ensures the text properly wraps within the available terminal width,
// with some special treatment for bullet points, for example a two line bullet point has
// extra indentation on the second line so it looks nice:
//   - Example of a wrapped
//     bullet point.
func formatMarkdownLine(markdown string, width int, startOfLinePrefix, wrappedLinePrefix string, style lipgloss.Style) []string {
	// how much width is taken up by the indentation prefixes (if any)
	prefixedWidth := max(lipgloss.Width(startOfLinePrefix), lipgloss.Width(wrappedLinePrefix))

	// then work out how much space we actually have.
	availableWidth := max(width-prefixedWidth, 1)

	// then we let Lipgloss wrap the text within that available width, with added hyperlinks, so they handle line breaks correctly.
	wrappedText := lipgloss.NewStyle().Width(availableWidth).Render(styleMarkdownLinks(markdown, style))
	textLines := strings.Split(wrappedText, "\n")

	out := make([]string, 0, len(textLines))

	for lineNumber, line := range textLines {
		// Pick the correct prefix to use.
		prefix := wrappedLinePrefix
		if lineNumber == 0 {
			prefix = startOfLinePrefix
		}
		line = strings.TrimRight(line, " ")

		// Add the chosen prefix, and go through and tag any links, to fix a rendering glitch.
		out = append(out, trimRight(prefix+tagHyperlinksWithLineNumber(line, lineNumber), width))
	}

	return out
}
