package github

import (
	"strings"
	"time"
)

// contextNode mirrors the union type returned for each check in the
// statusCheckRollup.contexts list. CheckRun fields and StatusContext fields
// are both populated; the Typename discriminates.
type contextNode struct {
	Typename    string `json:"__typename"`
	Name        string `json:"name"`        // CheckRun
	DetailsURL  string `json:"detailsUrl"`  // CheckRun
	StartedAt   string `json:"startedAt"`   // CheckRun
	CompletedAt string `json:"completedAt"` // CheckRun
	Context     string `json:"context"`     // StatusContext
	TargetURL   string `json:"targetUrl"`   // StatusContext
	CreatedAt   string `json:"createdAt"`   // StatusContext
	Status      string `json:"status"`      // CheckRun: QUEUED/IN_PROGRESS/COMPLETED/...
	Conclusion  string `json:"conclusion"`  // CheckRun: SUCCESS/FAILURE/...
	State       string `json:"state"`       // StatusContext: PENDING/SUCCESS/FAILURE/ERROR
}

type prNode struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	HeadRef    string `json:"headRefName"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	ReviewThreads struct {
		Nodes []struct {
			IsResolved bool `json:"isResolved"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []contextNode `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

func prFromNode(n prNode) PR {
	p := PR{
		Number:  n.Number,
		Title:   n.Title,
		URL:     n.URL,
		HeadRef: n.HeadRef,
		Repo:    n.Repository.NameWithOwner,
	}
	for _, t := range n.ReviewThreads.Nodes {
		if !t.IsResolved {
			p.Comments++
		}
	}
	if len(n.Commits.Nodes) == 0 || n.Commits.Nodes[0].Commit.StatusCheckRollup == nil {
		return p
	}
	// GitHub's rollup returns every check-run attempt, including stale
	// failures that have since been re-run. Keep only the newest attempt
	// per check name so a retry doesn't show as still failed.
	idxByName := map[string]int{}
	for _, c := range n.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
		ck := checkFromNode(c)
		if idx, ok := idxByName[ck.Name]; ok {
			if !ck.At.Before(p.Checks[idx].At) {
				p.Checks[idx] = ck
			}
			continue
		}
		idxByName[ck.Name] = len(p.Checks)
		p.Checks = append(p.Checks, ck)
	}
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

func checkFromNode(c contextNode) Check {
	ck := Check{State: StateNone}
	if c.Typename == "CheckRun" {
		ck.Name = c.Name
		ck.URL = c.DetailsURL
		if t := parseTS(c.CompletedAt); !t.IsZero() {
			ck.At = t
		} else if t := parseTS(c.StartedAt); !t.IsZero() {
			ck.At = t
		}
		switch strings.ToUpper(c.Status) {
		case "COMPLETED":
			switch strings.ToUpper(c.Conclusion) {
			case "SUCCESS", "NEUTRAL", "SKIPPED":
				ck.State = StateSuccess
			case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STALE", "STARTUP_FAILURE":
				ck.State = StateFailure
			}
		default:
			ck.State = StatePending
		}
	} else {
		ck.Name = c.Context
		ck.URL = c.TargetURL
		ck.At = parseTS(c.CreatedAt)
		switch strings.ToUpper(c.State) {
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
