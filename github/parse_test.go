package github

import (
	"fmt"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func ts(ago time.Duration) string {
	return time.Now().Add(-ago).UTC().Format(time.RFC3339)
}

// prJSON wraps check contexts in the commits.nodes[0].commit.statusCheckRollup
// shape GitHub returns.
func prJSON(extra string, contexts ...string) gjson.Result {
	list := ""
	for i, c := range contexts {
		if i > 0 {
			list += ","
		}
		list += c
	}
	return gjson.Parse(fmt.Sprintf(`{%s "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[%s]}}}}]}}`, extra, list))
}

func TestCheckFromJSON_CheckRunStates(t *testing.T) {
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
			n := gjson.Parse(fmt.Sprintf(
				`{"__typename":"CheckRun","name":"build","status":%q,"conclusion":%q}`,
				tc.status, tc.conclusion))
			if got := checkFromJSON(n).State; got != tc.want {
				t.Fatalf("status=%s conclusion=%s: got %s, want %s",
					tc.status, tc.conclusion, got, tc.want)
			}
		})
	}
}

func TestCheckFromJSON_StatusContextStates(t *testing.T) {
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
			n := gjson.Parse(fmt.Sprintf(
				`{"__typename":"StatusContext","context":"ci/legacy","state":%q}`, tc.state))
			if got := checkFromJSON(n).State; got != tc.want {
				t.Fatalf("state=%s: got %s, want %s", tc.state, got, tc.want)
			}
		})
	}
}

func TestCheckFromJSON_FieldMapping(t *testing.T) {
	completed := ts(5 * time.Minute)
	cr := checkFromJSON(gjson.Parse(fmt.Sprintf(
		`{"__typename":"CheckRun","name":"test","detailsUrl":"https://example/run/1","completedAt":%q,"status":"COMPLETED","conclusion":"SUCCESS"}`,
		completed)))
	if cr.Name != "test" || cr.URL != "https://example/run/1" || cr.At.IsZero() {
		t.Fatalf("CheckRun fields not mapped: %+v", cr)
	}

	sc := checkFromJSON(gjson.Parse(fmt.Sprintf(
		`{"__typename":"StatusContext","context":"ci/legacy","targetUrl":"https://example/build/2","createdAt":%q,"state":"SUCCESS"}`,
		completed)))
	if sc.Name != "ci/legacy" || sc.URL != "https://example/build/2" || sc.At.IsZero() {
		t.Fatalf("StatusContext fields not mapped: %+v", sc)
	}

	if checkFromJSON(gjson.Parse(`{"__typename":"CheckRun"}`)).Name != "(unnamed)" {
		t.Fatal("expected (unnamed) fallback")
	}
}

func TestPRFromJSON_RetryCollapsesOldAttempt(t *testing.T) {
	// A check that failed and is now being re-run: the rollup returns both
	// attempts. Only the newer one should count.
	n := prJSON(`"number":1,`,
		fmt.Sprintf(`{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE","completedAt":%q}`, ts(10*time.Minute)),
		fmt.Sprintf(`{"__typename":"CheckRun","name":"test","status":"IN_PROGRESS","startedAt":%q}`, ts(time.Minute)),
		fmt.Sprintf(`{"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"SUCCESS","completedAt":%q}`, ts(time.Minute)),
	)

	p := prFromJSON(n)
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

func TestPRFromJSON_UnresolvedComments(t *testing.T) {
	n := gjson.Parse(`{"number":7,"reviewThreads":{"nodes":[
		{"isResolved":false},{"isResolved":true},{"isResolved":false},{"isResolved":false}]}}`)
	if got := prFromJSON(n).Comments; got != 3 {
		t.Fatalf("Comments = %d, want 3", got)
	}
}

func TestPRFromJSON_NoRollup(t *testing.T) {
	// PRs with no commits/rollup (null) must parse without checks.
	for _, in := range []string{
		`{"number":1}`,
		`{"number":1,"commits":{"nodes":[]}}`,
		`{"number":1,"commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]}}`,
	} {
		if p := prFromJSON(gjson.Parse(in)); p.Total != 0 || p.State() != StateNone {
			t.Fatalf("%s: got %+v", in, p)
		}
	}
}

func TestPRFromJSON_AggregateCounts(t *testing.T) {
	n := prJSON(`"number":42,"title":"hi","url":"u","headRefName":"main","repository":{"nameWithOwner":"acme/api"},`,
		`{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"SUCCESS"}`,
		`{"__typename":"CheckRun","name":"b","status":"COMPLETED","conclusion":"FAILURE"}`,
		`{"__typename":"CheckRun","name":"c","status":"IN_PROGRESS"}`,
		`{"__typename":"StatusContext","context":"d","state":"PENDING"}`,
	)

	p := prFromJSON(n)
	if p.Total != 4 || p.Passed != 1 || p.Failed != 1 || p.Running != 2 {
		t.Fatalf("counts wrong: %+v", p)
	}
	if p.State() != StatePending {
		t.Fatalf("state should be pending, got %s", p.State())
	}
	if p.Title != "hi" || p.URL != "u" || p.HeadRef != "main" {
		t.Fatalf("fields not mapped: %+v", p)
	}
	if p.Key() != "acme/api#42" {
		t.Fatalf("key=%s", p.Key())
	}
}

func TestPR_Ready(t *testing.T) {
	green := PR{Total: 1, Passed: 1}
	cases := []struct {
		name string
		p    PR
		want bool
	}{
		{"clean-green", PR{Clean: true, Total: 1, Passed: 1}, true},
		{"clean-no-checks", PR{Clean: true}, true},
		{"not-clean", green, false},
		{"clean-failing", PR{Clean: true, Total: 1, Failed: 1}, false},
		{"clean-running", PR{Clean: true, Total: 1, Running: 1}, false},
		{"approved-but-blocked", PR{Approved: true, Total: 1, Passed: 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Ready(); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPRFromJSON_ApprovalAndMergeState(t *testing.T) {
	p := prFromJSON(gjson.Parse(`{"reviewDecision":"APPROVED","mergeStateStatus":"CLEAN"}`))
	if !p.Approved || !p.Clean {
		t.Fatalf("review/mergeStateStatus not parsed: %+v", p)
	}
	for _, st := range []string{"BLOCKED", "DIRTY", "BEHIND", "UNSTABLE", "DRAFT", ""} {
		if prFromJSON(gjson.Parse(`{"mergeStateStatus":"` + st + `"}`)).Clean {
			t.Fatalf("%q must not be clean", st)
		}
	}

	// A decision from GitHub wins over individual reviews.
	p = prFromJSON(gjson.Parse(`{"reviewDecision":"REVIEW_REQUIRED","latestOpinionatedReviews":{"nodes":[{"state":"APPROVED"}]}}`))
	if p.Approved {
		t.Fatal("REVIEW_REQUIRED must not count as approved")
	}

	// No decision (repo doesn't require reviews): fall back to actual reviews.
	cases := []struct {
		name   string
		states string
		want   bool
	}{
		{"approved", `[{"state":"APPROVED"}]`, true},
		{"approved-and-changes-requested", `[{"state":"APPROVED"},{"state":"CHANGES_REQUESTED"}]`, false},
		{"no-reviews", `[]`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := gjson.Parse(`{"reviewDecision":null,"latestOpinionatedReviews":{"nodes":` + tc.states + `}}`)
			if got := prFromJSON(n).Approved; got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
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
