// Package tui contains Flying Buttress terminal UI components built with
// Bubble Tea and Lip Gloss.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// shaWidth fits "sha256:" (7) + 8 hex chars from ShortHash.
const shaWidth = 15

var (
	// Brand palette — stone cathedral, warm amber accent.
	colorAccent  = lipgloss.Color("#C17D3C") // amber
	colorSubtle  = lipgloss.Color("#6B6B6B") // muted grey
	colorSuccess = lipgloss.Color("#5FAD56") // soft green
	colorError   = lipgloss.Color("#D64045") // muted red

	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	StyleSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	StyleDim = lipgloss.NewStyle().
			Foreground(colorSubtle)

	StyleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess)

	StyleError = lipgloss.NewStyle().
			Foreground(colorError)

	StyleSHA = lipgloss.NewStyle().
			Foreground(colorAccent).
			Width(shaWidth)

	StyleDesc = lipgloss.NewStyle().
			Foreground(colorSubtle)

	cursor   = StyleSelected.Render(">")
	noCursor = "  "
)

// ShortHash returns a display-friendly truncation of a content hash.
// "sha256:a1b2c3d4e5f6..." → "sha256:a1b2c3d4"
func ShortHash(h string) string {
	if idx := strings.Index(h, ":"); idx != -1 {
		alg := h[:idx+1]
		rest := h[idx+1:]
		if len(rest) > 8 {
			rest = rest[:8]
		}
		return alg + rest
	}
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
