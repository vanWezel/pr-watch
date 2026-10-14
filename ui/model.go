// Package ui implements the Bubble Tea TUI for prwatch.
package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vanWezel/pr-watch/github"
)

// Config holds runtime settings supplied by the caller.
type Config struct {
	Interval time.Duration
	Limit    int
	Notify   bool
}

type view int

const (
	viewList view = iota
	viewLogs
)

// Model is the Bubble Tea model. Exposed so main can pass it to tea.NewProgram.
type Model struct {
	cfg Config

	prs    []github.PR
	cursor int
	failed map[string]bool // PRs we've already notified about

	loading bool
	err     error

	view        view
	logsTitle   string
	logsBody    string
	logsLoading bool
	logsVP      viewport.Model

	width      int
	height     int
	spin       spinner.Model
	lastUpdate time.Time
}

// New constructs an initial Model.
func New(cfg Config) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colAccent)

	return Model{
		cfg:     cfg,
		failed:  map[string]bool{},
		loading: true,
		spin:    sp,
		logsVP:  viewport.New(80, 20),
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(m.cfg.Interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchPRsCmd(m.cfg.Limit), m.spin.Tick, m.tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.logsVP.Width = max(20, msg.Width-6)
		m.logsVP.Height = max(5, msg.Height-8)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case prsMsg:
		m.loading = false
		m.lastUpdate = time.Now()
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		if m.cfg.Notify {
			m.failed = notifyNewFailures(m.failed, msg.prs)
		} else {
			m.failed = map[string]bool{}
		}
		m.prs = msg.prs
		if m.cursor >= len(m.prs) {
			m.cursor = max(0, len(m.prs)-1)
		}
		return m, nil

	case logsMsg:
		m.logsLoading = false
		if msg.err != nil {
			m.logsBody = "Error: " + msg.err.Error()
		} else {
			m.logsBody = msg.body
		}
		m.logsVP.SetContent(m.logsBody)
		m.logsVP.GotoBottom()
		return m, nil

	case tickMsg:
		return m, tea.Batch(fetchPRsCmd(m.cfg.Limit), m.tick())

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	}

	if m.view == viewLogs {
		if msg.String() == "esc" {
			m.view = viewList
			return m, nil
		}
		var cmd tea.Cmd
		m.logsVP, cmd = m.logsVP.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "r":
		m.loading = true
		return m, fetchPRsCmd(m.cfg.Limit)
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.prs)-1 {
			m.cursor++
		}
	case "o", "enter":
		if len(m.prs) > 0 {
			return m, openURL(m.prs[m.cursor].URL)
		}
	case "a":
		if len(m.prs) > 0 {
			return m, openLatestRunCmd(m.prs[m.cursor])
		}
	case "l":
		if len(m.prs) > 0 {
			p := m.prs[m.cursor]
			m.view = viewLogs
			m.logsTitle = fmt.Sprintf("%s · #%d %s", p.Repo, p.Number, p.Title)
			m.logsBody = ""
			m.logsVP.SetContent("")
			m.logsLoading = true
			return m, tea.Batch(fetchLogsCmd(p), m.spin.Tick)
		}
	}
	return m, nil
}

// notifyNewFailures fires a desktop notification for each PR that newly
// transitioned to a failed state. Returns the new flagged-keys set.
func notifyNewFailures(prev map[string]bool, prs []github.PR) map[string]bool {
	next := map[string]bool{}
	for _, p := range prs {
		if p.State() != github.StateFailure {
			continue
		}
		if !prev[p.Key()] {
			notify(
				fmt.Sprintf("PR check failed · %s", p.Repo),
				fmt.Sprintf("#%d %s", p.Number, p.Title),
				p.URL,
			)
		}
		next[p.Key()] = true
	}
	return next
}
