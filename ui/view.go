package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vanWezel/pr-watch/github"
)

func (m Model) View() string {
	if m.view == viewLogs {
		return m.logsView()
	}
	return m.listView()
}

func (m Model) header() string {
	t := titleStyle.Render(" PR Watch ")
	sub := subtitleStyle.Render(fmt.Sprintf(" open pull requests · auto-refresh %s", m.cfg.Interval))
	return lipgloss.JoinHorizontal(lipgloss.Center, t, sub)
}

func (m Model) footer(keys [][2]string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, helpKey.Render(k[0])+" "+helpDesc.Render(k[1]))
	}
	help := strings.Join(parts, helpDesc.Render(" · "))
	if !m.lastUpdate.IsZero() {
		help += helpDesc.Render("    ") +
			subtitleStyle.Render(fmt.Sprintf("updated %s", m.lastUpdate.Format("15:04:05")))
	}
	return help
}

func (m Model) listView() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")

	if m.err != nil && len(m.prs) == 0 {
		b.WriteString(errStyle.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("Make sure `gh` is installed and authenticated (`gh auth login`)."))
		b.WriteString("\n\n")
		b.WriteString(m.footer([][2]string{{"r", "retry"}, {"q", "quit"}}))
		return b.String()
	}
	if m.err != nil {
		b.WriteString(errStyle.Render("⚠ " + m.err.Error()))
		b.WriteString(subtitleStyle.Render("  (showing last successful fetch — will retry)"))
		b.WriteString("\n\n")
	}

	outerW := max(40, m.width-2)
	box := borderStyle.Width(outerW)

	if m.loading && len(m.prs) == 0 {
		b.WriteString(box.Render(m.spin.View() + " loading pull requests…"))
		b.WriteString("\n\n")
		b.WriteString(m.footer([][2]string{{"q", "quit"}}))
		return b.String()
	}

	if len(m.prs) == 0 {
		b.WriteString(box.Render(subtitleStyle.Render("No open PRs found. ✨")))
		b.WriteString("\n\n")
		b.WriteString(m.footer([][2]string{{"r", "refresh"}, {"q", "quit"}}))
		return b.String()
	}

	innerW := max(40, outerW-6)
	rows := m.renderRows(innerW)
	listBody := strings.Join(rows, "\n")
	divider := lipgloss.NewStyle().Foreground(colSubtle).Render(strings.Repeat("─", innerW))
	full := listBody + "\n" + divider + "\n" + m.checksSection(innerW)

	b.WriteString(box.Render(full))
	b.WriteString("\n")
	b.WriteString(m.footer([][2]string{
		{"↑/↓", "move"},
		{"o", "open PR"},
		{"a", "open action"},
		{"l", "logs"},
		{"r", "refresh"},
		{"q", "quit"},
	}))
	return b.String()
}

func (m Model) renderRows(innerW int) []string {
	repoW := 0
	for _, p := range m.prs {
		if l := len(p.Repo); l > repoW {
			repoW = l
		}
	}
	const commentsW = 6 // "💬 99 " — emoji is 2 cells
	// Layout: [badge 14][gap 2][#num 6][gap 2][cmts 6][gap 2][repo repoW][gap 2][title rest]; -2 for cursor prefix.
	titleW := max(10, innerW-14-2-6-2-commentsW-2-repoW-2-2)

	rows := make([]string, 0, len(m.prs))
	for i, p := range m.prs {
		st := badge(p)
		num := hyperlink(p.URL, lipgloss.NewStyle().Foreground(colMuted).Render(fmt.Sprintf("#%-4d", p.Number)))
		cmt := commentsCell(p.Comments, commentsW)
		title := p.Title
		if len(title) > titleW {
			title = title[:titleW-1] + "…"
		}
		repoStyled := lipgloss.NewStyle().Foreground(colAccent).
			Render(fmt.Sprintf("%-*s", repoW, p.Repo))
		line := fmt.Sprintf("%s  %s  %s  %s  %s", st, num, cmt, repoStyled, title)
		if i == m.cursor {
			rows = append(rows, rowSelStyle.Width(innerW).Render("▸ "+line))
		} else {
			rows = append(rows, rowStyle.Width(innerW).Render("  "+line))
		}
	}
	return rows
}

func commentsCell(n int, width int) string {
	pad := lipgloss.NewStyle().Width(width)
	if n == 0 {
		return pad.Render("")
	}
	return pad.Render(lipgloss.NewStyle().Foreground(colPending).Render(fmt.Sprintf("💬 %d", n)))
}

func (m Model) checksSection(innerW int) string {
	if m.cursor >= len(m.prs) {
		return ""
	}
	p := m.prs[m.cursor]
	title := lipgloss.NewStyle().Foreground(colAccent).Bold(true).
		Render(fmt.Sprintf("Checks · #%d", p.Number))

	if len(p.Checks) == 0 {
		return title + "\n" + subtitleStyle.Render("(no checks reported)")
	}

	const tsW = 18 // "12:34:56 (5m ago) "
	nameW := max(10, innerW-2-tsW)

	rows := make([]string, 0, len(p.Checks))
	for _, c := range p.Checks {
		name := c.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "…"
		}
		nameStyled := name
		switch c.State {
		case github.StateFailure:
			nameStyled = lipgloss.NewStyle().Foreground(colFailure).Render(name)
		case github.StatePending:
			nameStyled = lipgloss.NewStyle().Foreground(colPending).Render(name)
		}
		nameStyled = hyperlink(c.URL, nameStyled)
		ts := subtitleStyle.Render(formatTS(c.At))
		nameCell := lipgloss.NewStyle().Width(nameW).Render(nameStyled)
		rows = append(rows, fmt.Sprintf("%s %s  %s", checkIcon(c.State), nameCell, ts))
	}

	return title + "\n\n" + strings.Join(rows, "\n")
}

func (m Model) logsView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Action logs "))
	b.WriteString("  ")
	b.WriteString(subtitleStyle.Render(m.logsTitle))
	b.WriteString("\n\n")

	w := max(40, m.width-2)
	if m.logsLoading {
		b.WriteString(borderStyle.Width(w).Render(m.spin.View() + " fetching logs…"))
	} else {
		b.WriteString(borderStyle.Width(w).Render(m.logsVP.View()))
	}
	b.WriteString("\n")
	pct := int(m.logsVP.ScrollPercent() * 100)
	b.WriteString(m.footer([][2]string{
		{"↑/↓", "scroll"},
		{"pgup/pgdn", "page"},
		{"esc", "back"},
		{"q", "quit"},
	}))
	b.WriteString(helpDesc.Render("    ") + subtitleStyle.Render(fmt.Sprintf("scroll %d%%", pct)))
	return b.String()
}
