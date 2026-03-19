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
prompt → spec → code → review → done → complete
(agent)  (user) (agent) (user)  (agent)
```

**The agent only acts when `approved: true`.** After each agent action it resets `approved` to `false` so you must explicitly re-approve the next stage.

| Status | Who acts | What you must set to proceed |
|---|---|---|
| `prompt` | Agent | Set `approved: true` to trigger spec generation |
| `spec` | **You** | Review spec → set `status: "code"` + `approved: true` |
| `code` | Agent | Set `approved: true` to trigger implementation |
| `review` | **You** | Review branch → set `status: "done"` + `approved: true` |
| `done` | Agent | Set `approved: true` to trigger PR creation |
| `complete` | — | Done |
| `failed` | **You** | Fix manually → set `status: "code"` + `approved: true` to retry |

## Task Format

Add tasks to `KANBAN.json` manually. Set `approved: true` to let the agent act:

```json
[
  {
    "id": 1,
    "title": "Add health check endpoint",
    "prompt": "Add a GET /health endpoint that returns {\"status\": \"ok\"} with HTTP 200",
    "spec": "",
    "branch": "",
    "status": "prompt",
    "approved": true
  }
]
```

The agent fills in `spec` and `branch` automatically, and resets `approved` to `false` after each stage.

## Key Files

| File | Purpose |
|---|---|
| `main.go` | Full implementation (single file, stdlib only) |
| `KANBAN.json` | The board — edit this to add tasks and approve lanes |

No `SPEC.md` needed — specs are written by the agent and stored inside `KANBAN.json`.

## Git & GitHub

- Branch name format: `feature/task-{id}-{title-slug}`
- PRs are raised via the `gh` CLI — ensure `gh auth login` has been run
- The PR body is auto-populated with the task spec

## Dependencies

- Go stdlib only (`encoding/json`, `os/exec`, `bufio`, `sync`, `time`, `flag`)
- `claude` CLI — must be installed and authenticated
- `gh` CLI — required for raising PRs (`gh auth login`)
- `git` — must be initialized in the working directory
