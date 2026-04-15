# prwatch

Terminal dashboard for your open GitHub pull requests, with live check status,
log inspection, and desktop notifications when a PR's checks turn red.

## Requirements

- [`gh`](https://cli.github.com) installed and authenticated (`gh auth login`)
- Go 1.21+ to build from source

## Install

```sh
make install        # installs to $GOBIN / $GOPATH/bin
# or
make build          # builds ./prwatch in this directory
```

## Usage

```sh
prwatch                                  # default: 60s refresh, 50 PRs each
prwatch -interval 30s -limit 25          # tune the polling
prwatch -no-notify                       # suppress desktop notifications
prwatch -version
```

The dashboard lists every open pull request you authored *or* are assigned to,
across all the repos your `gh` token can see, deduplicated by repo+number.

## Keys

| Key            | Action                                       |
| -------------- | -------------------------------------------- |
| `↑`/`↓`, `j`/`k` | Move selection                             |
| `o`, `Enter`   | Open the selected PR in your browser         |
| `a`            | Open the latest workflow run in your browser |
| `l`            | Show the latest action logs (scrollable)     |
| `r`            | Refresh now                                  |
| `Esc`          | Back from the logs view                      |
| `q`, `Ctrl+C`  | Quit                                         |

In the logs view, `↑`/`↓` and `PgUp`/`PgDn` scroll the buffer.

## Status badges

| Badge           | Meaning                                                             |
| --------------- | ------------------------------------------------------------------- |
| `● pass`        | Every check finished and is green.                                  |
| `● N run`       | At least one check is still running. `N✗` suffix counts failures.   |
| `● N fail`      | All checks finished and at least one failed.                        |
| `○ none`        | No checks reported yet on the latest commit.                        |

A desktop notification fires once per PR when it transitions into the `fail`
state. No notifications are sent for in-progress runs, even if a sibling
check has already failed.

## Notifications

- macOS: `osascript display notification` (Funk sound)
- Linux: `notify-send` (if available)
- Other: silent

## Development

```sh
make test           # race + coverage
make vet
make fmt
```
