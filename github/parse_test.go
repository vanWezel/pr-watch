package github

import (
	"testing"
	"time"
)

func TestCheckFromNode_CheckRunStates(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		conclusion string
		want       CheckState
	}{
		{"completed-success", "COMPLETED", "SUCCESS", StateSuccess},
		{"completed-neutral", "COMPLETED", "NEUTRAL", StateSuccess},
		{"completed-skipped", "COMPLETED", "SKIPPED", StateSuccess},
		{"completed-failure", "COMPLETED", "FAILURE", StateFailure},
		{"completed-cancelled", "COMPLETED", "CANCELLED", StateFailure},
		{"completed-timed-out", "COMPLETED", "TIMED_OUT", StateFailure},
		{"completed-startup-failure", "COMPLETED", "STARTUP_FAILURE", StateFailure},
		{"queued", "QUEUED", "", StatePending},
		{"in-progress", "IN_PROGRESS", "", StatePending},
		{"waiting", "WAITING", "", StatePending},
		{"unknown-status", "WHATEVER", "", StatePending},
		{"unknown-conclusion", "COMPLETED", "MYSTERY", StateNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := contextNode{
				Typename:   "CheckRun",
				Name:       "build",
				Status:     tc.status,
				Conclusion: tc.conclusion,
			}
			got := checkFromNode(n).State
			if got != tc.want {
				t.Fatalf("status=%s conclusion=%s: got %s, want %s",
					tc.status, tc.conclusion, got, tc.want)
			}
		})
	}
}

func TestCheckFromNode_StatusContextStates(t *testing.T) {
	cases := []struct {
		state string
		want  CheckState
	}{
		{"SUCCESS", StateSuccess},
		{"FAILURE", StateFailure},
		{"ERROR", StateFailure},
		{"PENDING", StatePending},
		{"EXPECTED", StatePending},
		{"weird", StateNone},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			n := contextNode{
				Typename: "StatusContext",
				Context:  "ci/legacy",
				State:    tc.state,
			}
			got := checkFromNode(n).State
			if got != tc.want {
				t.Fatalf("state=%s: got %s, want %s", tc.state, got, tc.want)
			}
		})
	}
}

func TestCheckFromNode_FieldMapping(t *testing.T) {
	completed := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	cr := checkFromNode(contextNode{
		Typename:    "CheckRun",
		Name:        "test",
		DetailsURL:  "https://example/run/1",
		CompletedAt: completed,
		Status:      "COMPLETED",
		Conclusion:  "SUCCESS",
	})
	if cr.Name != "test" || cr.URL != "https://example/run/1" || cr.At.IsZero() {
		t.Fatalf("CheckRun fields not mapped: %+v", cr)
	}

	sc := checkFromNode(contextNode{
		Typename:  "StatusContext",
		Context:   "ci/legacy",
		TargetURL: "https://example/build/2",
		CreatedAt: completed,
		State:     "SUCCESS",
	})
	if sc.Name != "ci/legacy" || sc.URL != "https://example/build/2" || sc.At.IsZero() {
		t.Fatalf("StatusContext fields not mapped: %+v", sc)
	}

	if checkFromNode(contextNode{Typename: "CheckRun"}).Name != "(unnamed)" {
		t.Fatal("expected (unnamed) fallback")
	}
}

func TestPRFromNode_RetryCollapsesOldAttempt(t *testing.T) {
	// Simulate a check that failed and is now being re-run: the rollup
	// returns both attempts. Only the newer one should count.
	old := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	newAttempt := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)

	n := prNode{Number: 1}
	var entry struct {
		Commit struct {
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []contextNode `json:"nodes"`
				} `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"commit"`
	}
	entry.Commit.StatusCheckRollup = &struct {
		Contexts struct {
			Nodes []contextNode `json:"nodes"`
		} `json:"contexts"`
	}{}
	entry.Commit.StatusCheckRollup.Contexts.Nodes = []contextNode{
		{Typename: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: "FAILURE", CompletedAt: old},
		{Typename: "CheckRun", Name: "test", Status: "IN_PROGRESS", StartedAt: newAttempt},
		{Typename: "CheckRun", Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS", CompletedAt: newAttempt},
	}
	n.Commits.Nodes = append(n.Commits.Nodes, entry)

	p := prFromNode(n)
	if p.Total != 2 {
		t.Fatalf("Total = %d, want 2 (retries should collapse)", p.Total)
	}
	if p.Failed != 0 || p.Running != 1 || p.Passed != 1 {
		t.Fatalf("counts wrong after retry: %+v", p)
	}
	if p.State() != StatePending {
		t.Fatalf("PR should be pending while retry in progress, got %s", p.State())
	}
	// The surviving "test" check should be the newer in-progress one.
	for _, c := range p.Checks {
		if c.Name == "test" && c.State != StatePending {
			t.Fatalf("retry kept wrong attempt: %+v", c)
		}
	}
}

func TestPRFromNode_UnresolvedComments(t *testing.T) {
	n := prNode{Number: 7}
	n.ReviewThreads.Nodes = []struct {
		IsResolved bool `json:"isResolved"`
	}{
		{IsResolved: false},
		{IsResolved: true},
		{IsResolved: false},
		{IsResolved: false},
	}
	if got := prFromNode(n).Comments; got != 3 {
		t.Fatalf("Comments = %d, want 3", got)
	}
}

func TestPRFromNode_AggregateCounts(t *testing.T) {
	n := prNode{Number: 42, Title: "hi", URL: "u", HeadRef: "main"}
	n.Repository.NameWithOwner = "acme/api"

	var entry struct {
		Commit struct {
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []contextNode `json:"nodes"`
				} `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"commit"`
	}
	entry.Commit.StatusCheckRollup = &struct {
		Contexts struct {
			Nodes []contextNode `json:"nodes"`
		} `json:"contexts"`
	}{}
	entry.Commit.StatusCheckRollup.Contexts.Nodes = []contextNode{
		{Typename: "CheckRun", Name: "a", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Typename: "CheckRun", Name: "b", Status: "COMPLETED", Conclusion: "FAILURE"},
		{Typename: "CheckRun", Name: "c", Status: "IN_PROGRESS"},
		{Typename: "StatusContext", Context: "d", State: "PENDING"},
	}
	n.Commits.Nodes = append(n.Commits.Nodes, entry)

	p := prFromNode(n)
	if p.Total != 4 || p.Passed != 1 || p.Failed != 1 || p.Running != 2 {
		t.Fatalf("counts wrong: %+v", p)
	}
	if p.State() != StatePending {
		t.Fatalf("state should be pending, got %s", p.State())
	}
	if p.Key() != "acme/api#42" {
		t.Fatalf("key=%s", p.Key())
	}
}

func TestPR_State(t *testing.T) {
	cases := []struct {
		name string
		p    PR
		want CheckState
	}{
		{"empty", PR{}, StateNone},
		{"all-pass", PR{Total: 2, Passed: 2}, StateSuccess},
		{"running-wins", PR{Total: 3, Passed: 1, Failed: 1, Running: 1}, StatePending},
		{"failed-when-done", PR{Total: 2, Passed: 1, Failed: 1}, StateFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.State(); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
