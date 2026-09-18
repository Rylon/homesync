package ui

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// hyperlinkOpens matches a basic OSC8 open sequence with any id parameter.
var hyperlinkOpens = regexp.MustCompile("\x1b]8;[^;]*;https://")

func TestFormatMarkdownLineLinksLabelAndURL(t *testing.T) {
	got := strings.Join(formatMarkdownLine("This is an example of a [Hyperlink](https://github.com), and some more text", 80, "", "", valueStyle), "\n")

	if plain := ansi.Strip(got); plain != "This is an example of a Hyperlink (https://github.com), and some more text" {
		t.Errorf("unexpected text: %q", plain)
	}
	if opens := len(hyperlinkOpens.FindAllString(got, -1)); opens != 2 {
		t.Errorf("label and URL should each be a hyperlink, got %d opens:\n%q", opens, got)
	}
}

func TestFormatMarkdownLineWrapsToWidth(t *testing.T) {
	text := "See [the project documentation](https://example.org/a/very/long/path/that/does/not/fit) for details."

	lines := formatMarkdownLine(text, 30, "", "", valueStyle)

	if len(lines) < 3 {
		t.Fatalf("expected the label and URL to wrap, got:\n%s", strings.Join(lines, "\n"))
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width > 30 {
			t.Errorf("line wider than 30: %q", ansi.Strip(line))
		}
		if opens, closes := len(hyperlinkOpens.FindAllString(line, -1)), strings.Count(line, ansi.ResetHyperlink()); opens != closes {
			t.Errorf("hyperlink not closed on its own line: %q", line)
		}
	}
	joined := strings.ReplaceAll(ansi.Strip(strings.Join(lines, "")), " ", "")
	if !strings.Contains(joined, "(https://example.org/a/very/long/path/that/does/not/fit)") || !strings.HasSuffix(joined, "details.") {
		t.Errorf("text was lost while wrapping:\n%s", strings.Join(lines, "\n"))
	}
}

func TestFormatMarkdownLinePreservesBlankLines(t *testing.T) {
	if got := formatMarkdownLine("", 40, "", "", valueStyle); len(got) != 1 || ansi.Strip(got[0]) != "" {
		t.Errorf("blank paragraph should render as one empty line, got %q", got)
	}
}

// Ultraviolet forgets that it has reset a hyperlink when it moves to a new row, so link text
// that wraps to a new line doesn't get-reopened, so we fix that by forcing a unique ID for
// each part of a link.
// See: https://github.com/charmbracelet/ultraviolet/issues/189
func TestHyperlinkIsReopenedOnTheNextLine(t *testing.T) {
	const url = "https://example.org"
	lines := formatMarkdownLine("A hyperlink [Hyperlink]("+url+"), and some more text", 20, "", "", valueStyle)

	var out bytes.Buffer
	renderer := uv.NewTerminalRenderer(&out, []string{"TERM=xterm-ghostty"})
	renderer.SetColorProfile(colorprofile.TrueColor)
	renderer.SetRelativeCursor(false)
	renderer.EnterAltScreen()
	screen := uv.NewScreenBuffer(20, len(lines))
	uv.NewStyledString(strings.Join(lines, "\n")).Draw(&screen, screen.Bounds())
	renderer.Render(screen.RenderBuffer)
	_ = renderer.Flush()

	withoutStyles := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(out.String(), "")
	openBeforeURL := regexp.MustCompile(regexp.QuoteMeta("\x1b]8;id=") + "[0-9]+;" + regexp.QuoteMeta(url+"\a(https://"))
	if !openBeforeURL.MatchString(withoutStyles) {
		t.Errorf("bracketed URL was written without reopening its hyperlink on its own line:\n%q", out.String())
	}
}
