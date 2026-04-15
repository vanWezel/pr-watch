// Package github wraps the `gh` CLI to fetch and inspect a user's open
// pull requests. It exposes only data types and pure functions; nothing
// in this package depends on the UI layer.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CmdTimeout bounds every gh invocation. It's well above normal API
// latency but short enough to avoid indefinite hangs when offline.
const CmdTimeout = 20 * time.Second

const maxLogLines = 2000

// runGH executes `gh <args...>` with a timeout and returns stdout.
// On failure it surfaces stderr inside the returned error.
func runGH(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, CmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("gh: timed out after %s", CmdTimeout)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.TrimSpace(string(ee.Stderr))
			if stderr == "" {
				stderr = err.Error()
			}
			return nil, fmt.Errorf("gh: %s", stderr)
		}
		return nil, fmt.Errorf("gh: %w", err)
	}
	return out, nil
}

// Preflight verifies that the gh CLI is installed and authenticated.
// The returned error is suitable to print before the UI starts.
func Preflight(ctx context.Context) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return errors.New("`gh` CLI not found in PATH. Install from https://cli.github.com")
	}
	authCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(authCtx, "gh", "auth", "status").CombinedOutput()
	if err != nil {
		return fmt.Errorf("`gh` is not authenticated (run `gh auth login`):\n%s",
			strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------- PR list ----------

const prFields = `nodes {
      ... on PullRequest {
        number
        title
        url
        headRefName
        repository { nameWithOwner }
        reviewThreads(first: 100) { nodes { isResolved } }
        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup {
                contexts(first: 100) {
                  nodes {
                    __typename
                    ... on CheckRun { name status conclusion detailsUrl startedAt completedAt }
                    ... on StatusContext { context state targetUrl createdAt }
                  }
                }
              }
            }
          }
        }
      }
    }`

func buildPRsQuery(limit int) string {
	l := strconv.Itoa(limit)
	return `query {
  authored: search(query: "is:pr is:open author:@me archived:false", type: ISSUE, first: ` + l + `) { ` + prFields + ` }
  assigned: search(query: "is:pr is:open assignee:@me archived:false", type: ISSUE, first: ` + l + `) { ` + prFields + ` }
}`
}

// FetchPRs returns the open PRs the user authored or is assigned to,
// across every repo their gh token can see. Results are deduplicated by
// repo+number.
func FetchPRs(ctx context.Context, limit int) ([]PR, error) {
	out, err := runGH(ctx, "api", "graphql", "-f", "query="+buildPRsQuery(limit))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Authored struct {
				Nodes []prNode `json:"nodes"`
			} `json:"authored"`
			Assigned struct {
				Nodes []prNode `json:"nodes"`
			} `json:"assigned"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse gh response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("gh graphql: %s", resp.Errors[0].Message)
	}
	seen := map[string]bool{}
	prs := make([]PR, 0)
	for _, n := range append(resp.Data.Authored.Nodes, resp.Data.Assigned.Nodes...) {
		p := prFromNode(n)
		if seen[p.Key()] {
			continue
		}
		seen[p.Key()] = true
		prs = append(prs, p)
	}
	return prs, nil
}

// FetchLogs returns the latest workflow log for a PR's branch. It first
// tries the failed-only log; if that's empty it falls back to the full log.
// The result is tail-trimmed to maxLogLines.
func FetchLogs(ctx context.Context, p PR) (string, error) {
	idOut, err := runGH(ctx, "run", "list",
		"--repo", p.Repo,
		"--branch", p.HeadRef,
		"--limit", "1",
		"--json", "databaseId",
		"-q", ".[0].databaseId",
	)
	if err != nil {
		return "", fmt.Errorf("locating run: %w", err)
	}
	runID := strings.TrimSpace(string(idOut))
	if runID == "" {
		return "(no workflow runs found for this branch)", nil
	}
	out, err := runGH(ctx, "run", "view", runID, "--repo", p.Repo, "--log-failed")
	if err != nil || len(out) == 0 {
		out, err = runGH(ctx, "run", "view", runID, "--repo", p.Repo, "--log")
		if err != nil {
			return "", err
		}
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > maxLogLines {
		lines = lines[len(lines)-maxLogLines:]
	}
	return strings.Join(lines, "\n"), nil
}

// LatestRunURL returns the GitHub URL of the latest workflow run for the
// PR's branch, falling back to the PR's /checks tab when no run is found
// or the lookup fails.
func LatestRunURL(ctx context.Context, p PR) string {
	out, err := runGH(ctx, "run", "list",
		"--repo", p.Repo,
		"--branch", p.HeadRef,
		"--limit", "1",
		"--json", "url",
		"-q", ".[0].url",
	)
	if err == nil {
		if url := strings.TrimSpace(string(out)); url != "" {
			return url
		}
	}
	return p.URL + "/checks"
}
