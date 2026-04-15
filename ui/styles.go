package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/vanWezel/pr-watch/github"
)

var (
	colMuted   = lipgloss.Color("245")
	colAccent  = lipgloss.Color("213")
	colSuccess = lipgloss.Color("84")
	colFailure = lipgloss.Color("203")
	colPending = lipgloss.Color("221")
	colSubtle  = lipgloss.Color("240")

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(colAccent).
			Bold(true).
			Padding(0, 2)

	subtitleStyle = lipgloss.NewStyle().Foreground(colMuted).Italic(true)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colSubtle).
			Padding(1, 2)

	rowStyle = lipgloss.NewStyle().Padding(0, 1)

	rowSelStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("237")).
			Foreground(lipgloss.Color("230")).
			Bold(true)

	helpKey  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	helpDesc = lipgloss.NewStyle().Foreground(colMuted)

	errStyle = lipgloss.NewStyle().Foreground(colFailure).Bold(true)
)

// badge renders the aggregate-check pill for a PR row.
func badge(p github.PR) string {
	const w = 14
	pad := lipgloss.NewStyle().Width(w)
	switch p.State() {
	case github.StateSuccess:
		return pad.Render(lipgloss.NewStyle().Foreground(colSuccess).Bold(true).Render("● pass"))
	case github.StateFailure:
		return pad.Render(lipgloss.NewStyle().Foreground(colFailure).Bold(true).Render(fmt.Sprintf("● %d fail", p.Failed)))
	case github.StatePending:
		base := lipgloss.NewStyle().Foreground(colPending).Bold(true).Render(fmt.Sprintf("● %d run", p.Running))
		if p.Failed > 0 {
			base += " " + lipgloss.NewStyle().Foreground(colFailure).Render(fmt.Sprintf("%d✗", p.Failed))
		}
		return pad.Render(base)
	default:
		return pad.Render(lipgloss.NewStyle().Foreground(colMuted).Render("○ none"))
	}
}

// checkIcon returns the single-glyph indicator used in the checks list.
func checkIcon(s github.CheckState) string {
	switch s {
	case github.StateSuccess:
		return lipgloss.NewStyle().Foreground(colSuccess).Render("✓")
	case github.StateFailure:
		return lipgloss.NewStyle().Foreground(colFailure).Render("✗")
	case github.StatePending:
		return lipgloss.NewStyle().Foreground(colPending).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(colMuted).Render("○")
	}
}

// hyperlink wraps text in an OSC 8 escape so terminals that support it
// render the text as a clickable link to url. Terminals that don't
// recognise OSC 8 simply ignore the sequence and show the bare text.
//
// SGR 24 ("underline off") is injected inside the link so terminals that
// honour text decoration within hyperlinks (e.g. iTerm2, kitty, wezterm)
// don't draw the default underline. Terminals that ignore SGR within
// OSC 8 (e.g. macOS Terminal.app) will still underline.
func hyperlink(url string, text string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + "\x1b[24m" + text + "\x1b[24m" + "\x1b]8;;\x1b\\"
}

// formatTS renders a wall-clock time plus a "Xm ago" relative hint.
func formatTS(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	var rel string
	switch {
	case d < 0:
		rel = "soon"
	case d < time.Minute:
		rel = fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		rel = fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		rel = fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		rel = fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return fmt.Sprintf("%s (%s)", t.Local().Format("15:04:05"), rel)
}
