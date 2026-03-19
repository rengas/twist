# twist

A CLI tool that runs a spec-driven, approval-gated Kanban workflow using the `claude` CLI.

## Build & Run

```bash
go build -o twist .
./twist -dir /path/to/your/project
```

Defaults to current directory if `-dir` is omitted.

## Kanban Lanes

```
prompt → spec → code → review → done
(agent)  (user) (agent+PR) (user)  (terminal)
```

**The agent only acts when `approved: true`.** After each agent action it resets `approved` to `false` so you must explicitly re-approve the next stage.

| Status | Who acts | What you must set to proceed |
|---|---|---|
| `prompt` | Agent | Set `approved: true` to trigger spec generation |
| `spec` | **You** | Review spec → set `status: "code"` + `approved: true` |
| `code` | Agent | Set `approved: true` to trigger implementation + PR creation |
| `review` | **You** | Review PR → approve to move to `done` |
| `done` | — | Terminal state |
| `failed` | **You** | Fix manually → set `status: "code"` + `approved: true` to retry |

## Task Format

Tasks are stored in a SQLite database (`twist.db`). Add tasks via the UI. The agent fills in `spec`, `branch`, and `pr_url` automatically, and resets `approved` to `false` after each stage.

After the agent finishes coding, it automatically pushes the branch, creates a PR via `gh`, and stores the PR URL on the task. The task then moves to `review` where the user can inspect the PR externally before approving.

## Key Files

| File | Purpose |
|---|---|
| `main.go` | Wails application entry point |
| `pkg/app.go` | Application struct, exposed methods, and background loop |
| `pkg/kanban.go` | Task struct, DB helpers, and lane handlers |
| `pkg/exports.go` | Exported wrappers for testing |
| `frontend/` | Vue 3 frontend |

## Git & GitHub

- Branch name format: `feature/task-{id}-{title-slug}`
- PRs are raised via the `gh` CLI — ensure `gh auth login` has been run
- The PR body is auto-populated with the task spec

## Dependencies

- Go stdlib only (`encoding/json`, `os/exec`, `bufio`, `sync`, `time`, `flag`)
- `claude` CLI — must be installed and authenticated
- `gh` CLI — required for raising PRs (`gh auth login`)
- `git` — must be initialized in the working directory
