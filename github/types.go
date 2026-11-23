package github

import (
	"fmt"
	"time"
)

// CheckState is the coarse status surfaced to the UI.
type CheckState string

const (
	StateSuccess CheckState = "SUCCESS"
	StateFailure CheckState = "FAILURE"
	StatePending CheckState = "PENDING"
	StateNone    CheckState = "NONE"
)

// Check is a single GitHub check (CheckRun or legacy StatusContext).
type Check struct {
	Name  string
	URL   string
	State CheckState
	At    time.Time // most recent activity (completed > started > created)
}

// PR is an open pull request with its check rollup.
type PR struct {
	Number   int
	Title    string
	URL      string
	HeadRef  string
	Repo     string // owner/name
	Checks   []Check
	Running  int
	Failed   int
	Passed   int
	Total    int
	Comments int // unresolved review threads

	Approved bool // reviewDecision == APPROVED
	Clean    bool // mergeStateStatus == CLEAN: GitHub sees nothing blocking the merge
}

// Ready reports whether GitHub considers the PR mergeable under its branch
// protection rules (reviews, required checks, conflicts, draft) and no check
// is running or failing.
func (p PR) Ready() bool {
	s := p.State()
	return p.Clean && (s == StateSuccess || s == StateNone)
}

// State derives a single status from the per-check counts. Running takes
// precedence so we don't flash "fail" while jobs are still in flight.
func (p PR) State() CheckState {
	if p.Total == 0 {
		return StateNone
	}
	if p.Running > 0 {
		return StatePending
	}
	if p.Failed > 0 {
		return StateFailure
	}
	return StateSuccess
}

// Key uniquely identifies a PR by repo and number.
func (p PR) Key() string {
	return fmt.Sprintf("%s#%d", p.Repo, p.Number)
}
