package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// Colour codes and escape sequences add bytes, but otherwise appear invisible,
// so we need to handle them properly.
func TestTrimKeepsStyledAndPlainTextToTheSameWidth(t *testing.T) {
	plain := strings.Repeat("abcdefghij", 8)
	styled := subtleStyle.Render("  • ") + valueStyle.Render(plain)

	for _, width := range []int{20, 40, 79, 200} {
		for name, text := range map[string]string{"plain": plain, "styled": styled} {
			for _, trim := range []struct {
				name string
				fn   func(string, int) string
			}{{"trimRight", trimRight}, {"trimLeft", trimLeft}} {
				got := trim.fn(text, width)

				if lipgloss.Width(got) > width {
					t.Errorf("%s(%s, %d) is %d wide", trim.name, name, width, lipgloss.Width(got))
				}
				if lipgloss.Width(text) > width && lipgloss.Width(got) != width {
					t.Errorf("%s(%s, %d) is %d wide, want the full %d", trim.name, name, width, lipgloss.Width(got), width)
				}
				if lipgloss.Width(text) <= width && got != text {
					t.Errorf("%s(%s, %d) changed text that already fit", trim.name, name, width)
				}
			}
		}
	}
}

func TestTrimAddsTheMarkerOnTheClippedSide(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog"

	if got := trimRight(text, 20); !strings.HasSuffix(got, "...") || !strings.HasPrefix(got, "the quick") {
		t.Errorf("trimRight = %q", got)
	}
	if got := trimLeft(text, 20); !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "lazy dog") {
		t.Errorf("trimLeft = %q", got)
	}
}
