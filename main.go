package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vanWezel/pr-watch/github"
	"github.com/vanWezel/pr-watch/ui"
)

// version is overridden at build time via `-ldflags "-X main.version=..."`.
var version = "dev"

func parseFlags(args []string) (ui.Config, error) {
	fs := flag.NewFlagSet("prwatch", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "prwatch %s — GitHub PR status dashboard\n\n", version)
		fmt.Fprintf(fs.Output(), "Usage: prwatch [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	interval := fs.Duration("interval", 60*time.Second, "refresh interval (min 5s)")
	limit := fs.Int("limit", 50, "max PRs per role to fetch (1-100)")
	noNotify := fs.Bool("no-notify", false, "disable desktop notifications on failure")
	showVer := fs.Bool("version", false, "print version and exit")

	if err := fs.Parse(args); err != nil {
		return ui.Config{}, err
	}
	if *showVer {
		fmt.Fprintln(os.Stdout, "prwatch", version)
		os.Exit(0)
	}
	if *interval < 5*time.Second {
		return ui.Config{}, fmt.Errorf("interval must be at least 5s, got %s", *interval)
	}
	if *limit < 1 || *limit > 100 {
		return ui.Config{}, fmt.Errorf("limit must be between 1 and 100, got %d", *limit)
	}
	return ui.Config{
		Interval: *interval,
		Limit:    *limit,
		Notify:   !*noNotify,
	}, nil
}

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "prwatch:", err)
		os.Exit(2)
	}

	if err := github.Preflight(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "prwatch:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(ui.New(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "prwatch:", err)
		os.Exit(1)
	}
}
