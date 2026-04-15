package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// notify sends a desktop notification on the user's platform. Failures are
// silent so a missing notifier never crashes the UI.
func notify(title string, body string) {
	switch runtime.GOOS {
	case "darwin":
		notifyDarwin(title, body)
	case "linux":
		notifyLinux(title, body)
	}
}

func notifyDarwin(title string, body string) {
	esc := func(s string) string { return strings.ReplaceAll(s, `"`, `\"`) }
	script := fmt.Sprintf(
		`display notification "%s" with title "%s" sound name "Funk"`,
		esc(body), esc(title),
	)
	_ = exec.Command("osascript", "-e", script).Start()
}

func notifyLinux(title string, body string) {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return
	}
	_ = exec.Command("notify-send", "--app-name=prwatch", title, body).Start()
}
