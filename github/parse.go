package github

import (
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// prFromJSON builds a PR from one `nodes` entry of the GraphQL search result.
func prFromJSON(n gjson.Result) PR {
	p := PR{
		Number:   int(n.Get("number").Int()),
		Title:    n.Get("title").String(),
		URL:      n.Get("url").String(),
		HeadRef:  n.Get("headRefName").String(),
		Repo:     n.Get("repository.nameWithOwner").String(),
		Clean:    n.Get("mergeStateStatus").String() == "CLEAN",
		Comments: int(n.Get(`reviewThreads.nodes.#(isResolved==false)#|#`).Int()),
	}

	if d := n.Get("reviewDecision").String(); d != "" {
		p.Approved = d == "APPROVED"
	} else {
		// No decision (repo doesn't require reviews): fall back to each
		// reviewer's latest review.
		approvals := n.Get(`latestOpinionatedReviews.nodes.#(state=="APPROVED")#|#`).Int()
		blocks := n.Get(`latestOpinionatedReviews.nodes.#(state=="CHANGES_REQUESTED")#|#`).Int()
		p.Approved = approvals > 0 && blocks == 0
	}

	contexts := n.Get("commits.nodes.0.commit.statusCheckRollup.contexts.nodes")
	if !contexts.Exists() {
		return p
	}
	// GitHub's rollup returns every check-run attempt, including stale
	// failures that have since been re-run. Keep only the newest attempt
	// per check name so a retry doesn't show as still failed.
	idxByName := map[string]int{}
	contexts.ForEach(func(_, c gjson.Result) bool {
		ck := checkFromJSON(c)
		if idx, ok := idxByName[ck.Name]; ok {
			if !ck.At.Before(p.Checks[idx].At) {
				p.Checks[idx] = ck
			}
			return true
		}
		idxByName[ck.Name] = len(p.Checks)
		p.Checks = append(p.Checks, ck)
		return true
	})
	for _, ck := range p.Checks {
		p.Total++
		switch ck.State {
		case StateSuccess:
			p.Passed++
		case StateFailure:
			p.Failed++
		case StatePending:
			p.Running++
		}
	}
	return p
}

// checkFromJSON maps a statusCheckRollup context, which is either a CheckRun
// or a legacy StatusContext (discriminated by __typename), to a Check.
func checkFromJSON(c gjson.Result) Check {
	ck := Check{State: StateNone}
	if c.Get("__typename").String() == "CheckRun" {
		ck.Name = c.Get("name").String()
		ck.URL = c.Get("detailsUrl").String()
		if t := parseTS(c.Get("completedAt").String()); !t.IsZero() {
			ck.At = t
		} else if t := parseTS(c.Get("startedAt").String()); !t.IsZero() {
			ck.At = t
		}
		switch strings.ToUpper(c.Get("status").String()) { // QUEUED/IN_PROGRESS/COMPLETED/...
		case "COMPLETED":
			switch strings.ToUpper(c.Get("conclusion").String()) {
			case "SUCCESS", "NEUTRAL", "SKIPPED":
				ck.State = StateSuccess
			case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STALE", "STARTUP_FAILURE":
				ck.State = StateFailure
			}
		default:
			ck.State = StatePending
		}
	} else {
		ck.Name = c.Get("context").String()
		ck.URL = c.Get("targetUrl").String()
		ck.At = parseTS(c.Get("createdAt").String())
		switch strings.ToUpper(c.Get("state").String()) {
		case "SUCCESS":
			ck.State = StateSuccess
		case "FAILURE", "ERROR":
			ck.State = StateFailure
		case "PENDING", "EXPECTED":
			ck.State = StatePending
		}
	}
	if ck.Name == "" {
		ck.Name = "(unnamed)"
	}
	return ck
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
