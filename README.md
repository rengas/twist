# Twist

A spec-driven, approval-gated Kanban workflow app powered by Claude. Twist automates the journey from idea to pull request — you describe a task, Claude generates a technical spec, implements the code, and raises a PR. You stay in control at every gate.

Built with [Wails](https://wails.io) (Go + Vue 3).

## How It Works

Tasks flow through a Kanban pipeline with human approval gates between agent-driven stages:

```
prompt  →  spec  →  code  →  review  →  done
(agent)   (user)   (agent)   (user)    (end)
```

1. **Prompt** — You write a task description. Approve it and Claude generates a detailed technical spec.
2. **Spec** — Review and edit the spec. Approve to start implementation.
3. **Code** — Claude implements the spec in an isolated git worktree, commits, pushes, and opens a PR via `gh`.
4. **Review** — Inspect the PR. Approve to mark as done.

The agent only acts when you set `approved: true`. After each stage it resets approval to `false`, so you always have explicit control.

## Features

- **Approval-gated workflow** — the agent never acts without your explicit sign-off
- **Parallel task processing** — configurable concurrency (up to 10 workers)
- **Git worktree isolation** — each task gets its own worktree so tasks don't interfere
- **Automatic PR creation** — pushes the branch and raises a PR with the spec as the body
- **Per-task chat** — discuss any task with Claude, with full session history
- **Shared design document** — cross-task context is maintained in a living `DESIGN.md`
- **Chat timeline** — unified view of workflow events and chat messages per task
- **PostgreSQL storage** — tasks, settings, chat history, and design versions persisted in Postgres
- **File-based migrations** — schema is managed with `golang-migrate`

## Prerequisites

- **Go** 1.25+
- **Node.js** (for the Vue 3 frontend)
- **Wails CLI** v2 — install with `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Claude CLI** — must be installed and authenticated
- **GitHub CLI (`gh`)** — must be authenticated (`gh auth login`)
- **Git** — initialized in the target project directory
- **PostgreSQL** — a running instance for task storage

## Setup

1. **Install Wails dependencies:**

   ```bash
   wails doctor
   ```

2. **Configure the database** (choose one):

   - Set the `TWIST_DATABASE_URL` environment variable, or
   - Connect via the UI on first launch, or
   - Create `~/.twist/config.json`:

     ```json
     {
       "database_url": "postgres://user:pass@localhost:5432/twist?sslmode=disable"
     }
     ```

3. **Run in development mode:**

   ```bash
   wails dev
   ```

4. **Build for production:**

   ```bash
   wails build
   ```

   The binary will be at `build/bin/twist`.

## Project Structure

```
main.go                  # Wails entry point
pkg/
  app.go                 # Application struct, exposed methods, background loop
  kanban.go              # Task model, lane handlers, Claude/git helpers
  repository.go          # Repository interface
  postgres.go            # PostgreSQL implementation
  config.go              # Config file (~/.twist/config.json)
  migrate.go             # File-based migrations with golang-migrate
  exports.go             # Exported wrappers for testing
  migrations/            # SQL migration files
frontend/
  src/
    App.vue              # Root Vue component
    components/
      KanbanBoard.vue    # Main board view
      TaskCard.vue       # Individual task card
      TaskModal.vue      # Task detail/edit modal
      AddTaskModal.vue   # New task creation
      ChatPanel.vue      # Per-task chat interface
      LogViewer.vue      # Real-time log output
      SettingsModal.vue  # App settings
      ConnectionModal.vue # Database connection setup
```

## Configuration

| Setting | Description | Default |
|---|---|---|
| Working directory | Target project for Claude to work in | Current directory |
| Max workers | Concurrent agent tasks (1–10) | 3 |
| Database URL | PostgreSQL connection string | — |

Settings are accessible from the UI and persisted in the database.

## License

MIT