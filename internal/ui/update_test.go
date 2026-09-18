package ui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Rylon/homesync/internal/update"
)

func releasedModel() Model {
	return Model{screen: screenDashboard, checker: update.Checker{Version: "0.1.0"}}
}

func offeredRelease() updateState {
	return updateState{checked: true, available: true, release: update.Release{Version: "0.2.0"}}
}

func TestHandleUpdateMsgTracksTheOutcome(t *testing.T) {
	cases := []struct {
		name          string
		msg           any
		wantAvailable bool
		wantApplied   bool
		wantErr       bool
	}{
		{"a newer version is offered", updateCheckedMsg{release: update.Release{Version: "0.2.0"}, found: true}, true, false, false},
		{"already on the latest version", updateCheckedMsg{found: false}, false, false, false},
		{"the check failed", updateCheckedMsg{err: errors.New("offline")}, false, false, true},
		{"the download succeeded", updateAppliedMsg{}, false, true, false},
		{"the download failed", updateAppliedMsg{err: errors.New("checksum mismatch")}, false, false, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := releasedModel()

			next, _ := model.handleUpdateMsg(testCase.msg)
			state := next.(Model).update

			if state.available != testCase.wantAvailable {
				t.Errorf("available = %v, want %v", state.available, testCase.wantAvailable)
			}

			if state.applied != testCase.wantApplied {
				t.Errorf("applied = %v, want %v", state.applied, testCase.wantApplied)
			}

			if (state.err != nil) != testCase.wantErr {
				t.Errorf("err = %v, wantErr %v", state.err, testCase.wantErr)
			}

			if state.applying {
				t.Error("the 'applying' state should be cleared once we have a result")
			}
		})
	}
}

// `U` opens the Update screen, but only if a new version was found.
func TestDashboardUpdateKeyOpensTheScreenOnlyWhenAnUpdateIsWaiting(t *testing.T) {
	cases := []struct {
		name       string
		state      updateState
		wantScreen screen
	}{
		{"nothing checked yet", updateState{}, screenDashboard},
		{"already on the latest version", updateState{checked: true}, screenDashboard},
		{"the check failed", updateState{checked: true, err: errors.New("offline")}, screenDashboard},
		{"a version is available", offeredRelease(), screenUpdate},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := releasedModel()
			model.update = testCase.state

			next, cmd := model.handleDashboardKey("U")

			if next.(Model).screen != testCase.wantScreen {
				t.Errorf("screen = %v, want %v", next.(Model).screen, testCase.wantScreen)
			}

			if cmd != nil {
				t.Error("opening the screen must not start the download command until the user confirms the update")
			}
		})
	}
}

// `enter` confirms the update, or retries if we got an error.
// `esc` rejects the update and returns to the dashboard.
func TestUpdateScreenKeys(t *testing.T) {
	applying := offeredRelease()
	applying.applying = true

	applied := offeredRelease()
	applied.applied = true

	failed := offeredRelease()
	failed.err = errors.New("HTTP 503")

	cases := []struct {
		name         string
		state        updateState
		key          string
		wantScreen   screen
		wantCmd      bool
		wantApplying bool
	}{
		{"enter starts the download", offeredRelease(), "enter", screenUpdate, true, true},
		{"enter again while downloading does nothing", applying, "enter", screenUpdate, false, true},
		{"enter after installing does nothing", applied, "enter", screenUpdate, false, false},
		{"enter after a failure tries again", failed, "enter", screenUpdate, true, true},
		{"esc goes back", offeredRelease(), "esc", screenDashboard, false, false},
		{"esc goes back while downloading, the download carries on", applying, "esc", screenDashboard, false, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := releasedModel()
			model.screen = screenUpdate
			model.update = testCase.state

			next, cmd := model.handleUpdateKey(testCase.key)
			got := next.(Model)

			if got.screen != testCase.wantScreen {
				t.Errorf("screen = %v, want %v", got.screen, testCase.wantScreen)
			}

			if (cmd != nil) != testCase.wantCmd {
				t.Errorf("cmd != nil is %v, want %v", cmd != nil, testCase.wantCmd)
			}

			if got.update.applying != testCase.wantApplying {
				t.Errorf("applying = %v, want %v", got.update.applying, testCase.wantApplying)
			}
		})
	}
}

func TestRenderReleaseNotesHandlesMarkdownAndLength(t *testing.T) {
	notes := "## Changelog\n* first change\n- second change\nplain line\n"

	got := renderReleaseNotes(notes, 80, 10)

	if len(got) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(got), strings.Join(got, "\n"))
	}
	if strings.Contains(got[0], "#") {
		t.Errorf("heading kept its markdown hashes: %q", got[0])
	}
	for _, line := range got[1:3] {
		if !strings.Contains(line, "•") {
			t.Errorf("bullet was not converted: %q", line)
		}
	}

	if got := renderReleaseNotes("", 80, 10); len(got) != 1 || !strings.Contains(got[0], "No release notes") {
		t.Errorf("empty notes rendered as %q", got)
	}

	if got := renderReleaseNotes(strings.Repeat("line\n", 30), 80, 5); len(got) != 5 || !strings.Contains(got[4], "truncated") {
		t.Errorf("long notes were not capped at 5 lines with a marker:\n%s", strings.Join(got, "\n"))
	}
}

func TestRenderReleaseNotesWrapsLongLinesInsteadOfTruncating(t *testing.T) {
	long := "This is a very long line in a fake set of release notes, which is used to test that word-wrapping works properly, rather than each line being truncated."
	notes := long + "\n\n* " + long + "\n"

	got := renderReleaseNotes(notes, 60, 40)

	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "...") {
		t.Errorf("notes were truncated instead of wrapped:\n%s", joined)
	}
	for _, line := range got {
		if width := lipgloss.Width(line); width > 60 {
			t.Errorf("line is %d wide, wider than 60: %q", width, line)
		}
	}
	plain := ansi.Strip(joined)
	for _, word := range strings.Fields(long) {
		if !strings.Contains(plain, word) {
			t.Errorf("wrapped notes lost the word %q:\n%s", word, plain)
		}
	}

	// The bullet points continuation lines line up with the text after the bullet, so wrapped
	// bullets read as one item.
	var bulletIndex int
	for index, line := range got {
		if strings.Contains(line, "•") {
			bulletIndex = index
		}
	}
	if bulletIndex == 0 || bulletIndex == len(got)-1 {
		t.Fatalf("expected a wrapped bullet with a continuation line:\n%s", plain)
	}
	continuation := ansi.Strip(got[bulletIndex+1])
	if !strings.HasPrefix(continuation, "    ") || strings.HasPrefix(continuation, "     ") {
		t.Errorf("bullet continuation should be indented by four spaces: %q", continuation)
	}
	if strings.Contains(continuation, "•") {
		t.Errorf("bullet continuation should not repeat the bullet: %q", continuation)
	}
}

func TestRenderReleaseNotesCapsHeightAfterWrapping(t *testing.T) {
	long := strings.Repeat("word ", 40)

	got := renderReleaseNotes(long, 40, 3)

	if len(got) != 3 || !strings.Contains(got[2], "truncated") {
		t.Errorf("wrapped notes were not capped at 3 lines with a marker:\n%s", strings.Join(got, "\n"))
	}
}

func TestRenderReleaseNotesBreaksUnbreakableTokens(t *testing.T) {
	url := "https://github.com/Rylon/homesync/releases/tag/v0.9.0/some/very/long/path/that/keeps/going"

	got := renderReleaseNotes(url, 30, 10)

	if len(got) < 2 {
		t.Fatalf("long URL should hard wrap onto several lines, got %d:\n%s", len(got), strings.Join(got, "\n"))
	}
	if strings.Join(ansiStripAll(got), "") != url {
		t.Errorf("hard wrapping lost characters from the URL:\n%s", strings.Join(got, "\n"))
	}
}

func ansiStripAll(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, ansi.Strip(line))
	}
	return out
}

func TestRenderReleaseNotesNormalisesWindowsLineEndings(t *testing.T) {
	// GitHub stores release bodies with CRLF line endings. A stray carriage return in a
	// rendered line makes the terminal renderer return to column zero, so the trailing
	// padding overwrites the start of the line.
	notes := "First paragraph that is long enough to wrap onto a second line when rendered.\r\n\r\n* A bullet point.\r\n"

	got := renderReleaseNotes(notes, 50, 20)

	for _, line := range got {
		if strings.Contains(line, "\r") {
			t.Errorf("rendered line still contains a carriage return: %q", line)
		}
	}
	plain := ansi.Strip(strings.Join(got, "\n"))
	if !strings.Contains(plain, "second line when rendered.") || !strings.Contains(plain, "A bullet point.") {
		t.Errorf("notes lost content:\n%s", plain)
	}
	if len(got) != 4 {
		t.Errorf("expected 4 lines (two wrapped, blank, bullet), got %d:\n%s", len(got), plain)
	}
}
