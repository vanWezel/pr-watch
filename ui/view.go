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

	// Vertical budget: header block (2) + box border/padding (4) + footer
	// line (1) + divider (1) = 8 fixed lines; split what's left between the
	// PR list and the selected PR's checks section so both fit on screen.
	avail := max(4, m.height-8)
	checksWant := 2
	if n := len(m.prs[m.cursor].Checks); n > 0 {
		checksWant = 2 + min(n, 8)
	}
	listMax := avail - checksWant
	if listMax < 1 {
		listMax = 1
	}
	if listMax > len(m.prs) {
		listMax = len(m.prs)
	}
	checksMax := avail - listMax

	allRows := m.renderRows(innerW)
	start, end := visibleWindow(len(allRows), m.cursor, listMax)
	listBody := strings.Join(allRows[start:end], "\n")
	divider := lipgloss.NewStyle().Foreground(colSubtle).Render(strings.Repeat("─", innerW))
	full := listBody + "\n" + divider + "\n" + m.checksSection(innerW, checksMax-2)

	b.WriteString(box.Render(full))
	b.WriteString("\n")
	ft := m.footer([][2]string{
		{"↑/↓", "move"},
		{"o", "open PR"},
		{"a", "open action"},
		{"l", "logs"},
		{"r", "refresh"},
		{"q", "quit"},
	})
	if len(m.prs) > 1 {
		ft += helpDesc.Render(fmt.Sprintf("    %d/%d", m.cursor+1, len(m.prs)))
	}
	b.WriteString(ft)
	return b.String()
}

// visibleWindow returns the [start,end) slice bounds of a maxVisible-sized
// window into a total-length list, centred on cursor so it stays in view.
func visibleWindow(total, cursor, maxVisible int) (start, end int) {
	if maxVisible >= total {
		return 0, total
	}
	start = cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	end = start + maxVisible
	if end > total {
		end = total
		start = end - maxVisible
	}
	return start, end
}

func (m Model) renderRows(innerW int) []string {
	repoW := 0
	for _, p := range m.prs {
		if l := len(p.Repo); l > repoW {
			repoW = l
		}
	}
	// Cap the repo column so a long "owner/repo" name can't crowd out the
	// title on narrow terminals; it scales with the box width.
	if repoCap := max(6, innerW/5); repoW > repoCap {
		repoW = repoCap
	}

	const commentsW = 6 // "💬 99 " — emoji is 2 cells
	const numW = 7      // "#%-4d" + " " + approved marker
	// Layout: [cursor 2][badge 14][gap 2][#num numW][gap 2][cmts commentsW][gap 2][repo repoW][gap 2][title rest]
	fixed := 2 + 14 + 2 + numW + 2 + commentsW + 2 + repoW + 2
	titleW := max(1, innerW-fixed)

	rows := make([]string, 0, len(m.prs))
	for i, p := range m.prs {
		st := badge(p)
		num := hyperlink(p.URL, lipgloss.NewStyle().Foreground(colMuted).Render(fmt.Sprintf("#%-4d", p.Number))) + " " + approvedMark(p)
		cmt := commentsCell(p.Comments, commentsW)
		title := p.Title
		if len(title) > titleW {
			title = title[:max(0, titleW-1)] + "…"
		}
		repo := p.Repo
		if len(repo) > repoW {
			repo = repo[:max(0, repoW-1)] + "…"
		}
		repoStyled := lipgloss.NewStyle().Foreground(colAccent).
			Render(fmt.Sprintf("%-*s", repoW, repo))
		line := fmt.Sprintf("%s  %s  %s  %s  %s", st, num, cmt, repoStyled, title)
		if i == m.cursor {
			rows = append(rows, rowSelStyle.Width(innerW).Render("▸ "+line))
		} else {
			rows = append(rows, rowStyle.Width(innerW).Render("  "+line))
		}
	}
	return rows
}

// approvedMark is a one-cell green check for approved PRs, blank otherwise,
// so the column stays aligned.
func approvedMark(p github.PR) string {
	if !p.Approved {
		return " "
	}
	return lipgloss.NewStyle().Foreground(colSuccess).Bold(true).Render("✔")
}

func commentsCell(n int, width int) string {
	pad := lipgloss.NewStyle().Width(width)
	if n == 0 {
		return pad.Render("")
	}
	return pad.Render(lipgloss.NewStyle().Foreground(colPending).Render(fmt.Sprintf("💬 %d", n)))
}

func (m Model) checksSection(innerW int, maxRows int) string {
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

	shown := p.Checks
	truncated := false
	if maxRows <= 0 {
		shown = nil
		truncated = true
	} else if len(shown) > maxRows {
		shown = shown[:maxRows-1]
		truncated = true
	}

	rows := make([]string, 0, len(shown)+1)
	for _, c := range shown {
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
	if truncated {
		rows = append(rows, subtitleStyle.Render(fmt.Sprintf("  … %d more", len(p.Checks)-len(shown))))
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
