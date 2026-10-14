package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// notify sends a desktop notification on the user's platform. When url is
// non-empty, clicking the notification opens it where the platform supports
// that. Failures are silent so a missing notifier never crashes the UI.
func notify(title string, body string, url string) {
	switch runtime.GOOS {
	case "darwin":
		notifyDarwin(title, body, url)
	case "linux":
		notifyLinux(title, body, url)
	}
}

func notifyDarwin(title string, body string, url string) {
	// osascript notifications can't carry a click action, so prefer
	// terminal-notifier when it's installed.
	if path, err := exec.LookPath("terminal-notifier"); err == nil {
		args := []string{"-title", title, "-message", body, "-sound", "Funk", "-group", "prwatch"}
		if url != "" {
			args = append(args, "-open", url)
		}
		_ = exec.Command(path, args...).Start()
		return
	}
	esc := func(s string) string { return strings.ReplaceAll(s, `"`, `\"`) }
	script := fmt.Sprintf(
		`display notification "%s" with title "%s" sound name "Funk"`,
		esc(body), esc(title),
	)
	_ = exec.Command("osascript", "-e", script).Start()
}

func notifyLinux(title string, body string, url string) {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		return
	}
	args := []string{"--app-name=prwatch"}
	if url == "" {
		_ = exec.Command(path, append(args, title, body)...).Start()
		return
	}
	// --wait blocks until the notification is dismissed and prints the
	// chosen action, so run it off the UI goroutine.
	args = append(args, "--wait", "--action=default=Open PR", title, body)
	go func() {
		out, err := exec.Command(path, args...).Output()
		if err == nil && strings.TrimSpace(string(out)) == "default" {
			_ = exec.Command("xdg-open", url).Start()
		}
	}()
}
