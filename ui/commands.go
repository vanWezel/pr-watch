package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vanWezel/pr-watch/github"
)

// Bubble Tea messages produced by the commands below.
type prsMsg struct {
	prs []github.PR
	err error
}

type logsMsg struct {
	key  string
	body string
	err  error
}

type tickMsg time.Time

// fetchPRsCmd wraps github.FetchPRs in a tea.Cmd.
func fetchPRsCmd(limit int) tea.Cmd {
	return func() tea.Msg {
		prs, err := github.FetchPRs(context.Background(), limit)
		return prsMsg{prs: prs, err: err}
	}
}

// fetchLogsCmd wraps github.FetchLogs in a tea.Cmd.
func fetchLogsCmd(p github.PR) tea.Cmd {
	return func() tea.Msg {
		body, err := github.FetchLogs(context.Background(), p)
		return logsMsg{key: p.Key(), body: body, err: err}
	}
}

// openLatestRunCmd resolves the latest workflow URL and opens it.
func openLatestRunCmd(p github.PR) tea.Cmd {
	return func() tea.Msg {
		url := github.LatestRunURL(context.Background(), p)
		return openURL(url)()
	}
}
