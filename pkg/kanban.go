package pkg

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Task represents a card on the Kanban board.
// Status flow: prompt → spec → code → review → done → complete
//
//	(agent)       (user)  (agent)  (user) (agent)
//
// The agent only acts when Approved is true. After each agent action it resets
// Approved to false so the user must explicitly re-approve the next stage.
type Task struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Prompt   string `json:"prompt"`   // user writes this
	Spec     string `json:"spec"`     // agent writes this
	Branch   string `json:"branch"`   // agent sets this
	Status   string `json:"status"`   // prompt | spec | code | review | done | complete | failed
	Approved bool   `json:"approved"` // user sets true to let agent act; agent resets to false after each stage
}

// LogFunc is called for each log line emitted during agent execution.
type LogFunc func(msg string)

// ── Database helpers ───────────────────────────────────────────────────────────

func dbPath(dir string) string {
	if dir == "" || dir == "." {
		return "twist.db"
	}
	return filepath.Join(dir, "twist.db")
}

const schema = `
CREATE TABLE IF NOT EXISTS tasks (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    title    TEXT    NOT NULL DEFAULT '',
    prompt   TEXT    NOT NULL DEFAULT '',
    spec     TEXT    NOT NULL DEFAULT '',
    branch   TEXT    NOT NULL DEFAULT '',
    status   TEXT    NOT NULL DEFAULT 'prompt',
    approved INTEGER NOT NULL DEFAULT 0
);`

const settingsSchema = `
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);`

func openDB(dir string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath(dir))
	if err != nil {
		return nil, err
	}
	// Serialize all writes through one connection; SQLite allows only one writer at a time.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return nil, err
	}
	// Wait up to 5 s before returning SQLITE_BUSY on write contention.
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return nil, err
	}
	if err := createSchema(db); err != nil {
		return nil, err
	}
	return db, nil
}

func createSchema(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	_, err := db.Exec(settingsSchema)
	return err
}

func getSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func setSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?,?)`, key, value)
	return err
}

func scanTask(row *sql.Row) (Task, error) {
	var t Task
	var approved int
	err := row.Scan(&t.ID, &t.Title, &t.Prompt, &t.Spec, &t.Branch, &t.Status, &approved)
	t.Approved = approved == 1
	return t, err
}

func loadTasks(db *sql.DB) ([]Task, error) {
	rows, err := db.Query(`SELECT id, title, prompt, spec, branch, status, approved FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		var approved int
		if err := rows.Scan(&t.ID, &t.Title, &t.Prompt, &t.Spec, &t.Branch, &t.Status, &approved); err != nil {
			return nil, err
		}
		t.Approved = approved == 1
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func insertTask(db *sql.DB, t Task) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO tasks (title, prompt, spec, branch, status, approved) VALUES (?,?,?,?,?,?)`,
		t.Title, t.Prompt, t.Spec, t.Branch, t.Status, boolToInt(t.Approved),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateTaskStatus(db *sql.DB, id int, status string, approved bool) error {
	_, err := db.Exec(
		`UPDATE tasks SET status=?, approved=? WHERE id=?`,
		status, boolToInt(approved), id,
	)
	return err
}

func updateTaskSpec(db *sql.DB, id int, spec string) error {
	_, err := db.Exec(`UPDATE tasks SET spec=? WHERE id=?`, spec, id)
	return err
}

func updateTaskBranch(db *sql.DB, id int, branch string) error {
	_, err := db.Exec(`UPDATE tasks SET branch=? WHERE id=?`, branch, id)
	return err
}

func deleteTask(db *sql.DB, id int) error {
	_, err := db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func findActionableDB(db *sql.DB) (Task, bool, error) {
	row := db.QueryRow(
		`SELECT id, title, prompt, spec, branch, status, approved FROM tasks
         WHERE approved=1 AND status IN ('prompt','code','done')
         ORDER BY id LIMIT 1`,
	)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	return t, err == nil, err
}

// ── JSON Migration ─────────────────────────────────────────────────────────────

func migrateFromJSON(dir string, db *sql.DB) {
	jsonPath := filepath.Join(dir, "KANBAN.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return // no JSON file, nothing to migrate
	}
	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&count)
	if count > 0 {
		os.Rename(jsonPath, jsonPath+".bak")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		return
	}
	for _, t := range tasks {
		tx.Exec(
			`INSERT INTO tasks (id, title, prompt, spec, branch, status, approved) VALUES (?,?,?,?,?,?,?)`,
			t.ID, t.Title, t.Prompt, t.Spec, t.Branch, t.Status, boolToInt(t.Approved),
		)
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return
	}
	os.Rename(jsonPath, jsonPath+".bak")
}

// ── Lane Handlers ─────────────────────────────────────────────────────────────

// handlePrompt: reads the user's prompt, writes a spec, moves task to 'spec' lane.
func handlePrompt(task Task, workDir string, db *sql.DB, log LogFunc) error {
	log(fmt.Sprintf("[SPEC] Task #%d: Writing spec for — %s", task.ID, task.Title))

	prompt := fmt.Sprintf(`You are a software architect. A user has the following request:

%s

Write a detailed technical specification as plain markdown. Include:
- Overview and goals
- Technical requirements
- File/directory structure
- Implementation steps
- Acceptance criteria

Return ONLY the markdown spec, no other commentary.`, task.Prompt)

	spec, err := runClaudeCapture(task.ID, prompt, workDir, log)
	if err != nil {
		return fmt.Errorf("spec generation failed for task #%d: %v", task.ID, err)
	}

	if err := updateTaskSpec(db, task.ID, strings.TrimSpace(spec)); err != nil {
		return err
	}
	if err := updateTaskStatus(db, task.ID, "spec", false); err != nil {
		return err
	}

	log(fmt.Sprintf("[SPEC READY] Task #%d: Spec written. Set status → 'code' + approved → true to approve.", task.ID))
	return nil
}

// handleCode: creates a git branch, runs claude to implement and commit, moves task to 'review' lane.
func handleCode(task Task, workDir string, db *sql.DB, log LogFunc) error {
	log(fmt.Sprintf("[CODE] Task #%d: Implementing — %s", task.ID, task.Title))

	branch := fmt.Sprintf("feature/task-%d-%s", task.ID, slugify(task.Title))

	log(fmt.Sprintf("[GIT] Creating branch: %s", branch))
	if _, err := gitCmd(workDir, "checkout", "-b", branch); err != nil {
		if _, err2 := gitCmd(workDir, "checkout", branch); err2 != nil {
			return fmt.Errorf("could not create or switch to branch %s: %v", branch, err2)
		}
	}

	if err := updateTaskBranch(db, task.ID, branch); err != nil {
		return err
	}

	prompt := fmt.Sprintf(`Implement the following specification exactly as written.
Run tests after implementation. Commit all changes with a descriptive commit message.

Specification:
%s

The git branch '%s' is already checked out. Do NOT create a new branch.
Do not ask for permission — implement, test, and commit.`, task.Spec, branch)

	if err := runClaudeStream(task.ID, prompt, workDir, log); err != nil {
		log(fmt.Sprintf("[FAILED] Task #%d implementation error: %v", task.ID, err))
		log("         Fix the issue manually, then set status → 'code' + approved → true to retry.")
		_ = updateTaskStatus(db, task.ID, "failed", false)
		return nil
	}

	if err := updateTaskStatus(db, task.ID, "review", false); err != nil {
		return err
	}

	log(fmt.Sprintf("[REVIEW READY] Task #%d: Code committed on branch '%s'. Set status → 'done' + approved → true to raise a PR.", task.ID, branch))
	return nil
}

// handleDone: pushes the branch and raises a PR, moves task to 'complete'.
func handleDone(task Task, workDir string, db *sql.DB, log LogFunc) error {
	log(fmt.Sprintf("[PR] Task #%d: Pushing branch and raising PR — %s", task.ID, task.Title))

	log(fmt.Sprintf("[GIT] Pushing branch: %s", task.Branch))
	if out, err := gitCmd(workDir, "push", "-u", "origin", task.Branch); err != nil {
		return fmt.Errorf("failed to push branch: %v\n%s", err, out)
	}

	prBody := fmt.Sprintf("## Task #%d: %s\n\n### Spec\n\n%s\n\n---\n🤖 Raised by twist",
		task.ID, task.Title, task.Spec)

	out, err := ghCmd(workDir, "pr", "create",
		"--title", fmt.Sprintf("Task #%d: %s", task.ID, task.Title),
		"--body", prBody,
	)
	if err != nil {
		return fmt.Errorf("failed to create PR: %v\n%s", err, out)
	}

	log(fmt.Sprintf("[PR RAISED] %s", strings.TrimSpace(out)))

	if err := updateTaskStatus(db, task.ID, "complete", false); err != nil {
		return err
	}

	log(fmt.Sprintf("[COMPLETE] Task #%d done — PR raised, task closed.", task.ID))
	return nil
}

// ── Claude Helpers ────────────────────────────────────────────────────────────

func runClaudeCapture(taskID int, prompt string, workDir string, log LogFunc) (string, error) {
	cmd := exec.Command("claude", "-p", "--dangerously-skip-permissions", prompt)
	cmd.Dir = workDir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start claude: %v", err)
	}

	stop := heartbeat(taskID, log)

	var (
		mu  sync.Mutex
		buf strings.Builder
		wg  sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			log(fmt.Sprintf("[CLAUDE] %s", line))
			mu.Lock()
			buf.WriteString(line + "\n")
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			log(fmt.Sprintf("[CLAUDE ERR] %s", scanner.Text()))
		}
	}()

	wg.Wait()
	close(stop)
	return buf.String(), cmd.Wait()
}

func runClaudeStream(taskID int, prompt string, workDir string, log LogFunc) error {
	cmd := exec.Command("claude", "-p", "--dangerously-skip-permissions", "--verbose", prompt)
	cmd.Dir = workDir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start claude: %v", err)
	}

	stop := heartbeat(taskID, log)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			log(fmt.Sprintf("[CLAUDE] %s", scanner.Text()))
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			log(fmt.Sprintf("[CLAUDE ERR] %s", scanner.Text()))
		}
	}()

	wg.Wait()
	close(stop)
	return cmd.Wait()
}

func heartbeat(taskID int, log LogFunc) chan struct{} {
	stop := make(chan struct{})
	go func() {
		start := time.Now()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log(fmt.Sprintf("[HEARTBEAT] Task #%d still running... (%ds elapsed)",
					taskID, int(time.Since(start).Seconds())))
			case <-stop:
				return
			}
		}
	}()
	return stop
}

// ── Git / GH Helpers ──────────────────────────────────────────────────────────

func gitCmd(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func ghCmd(workDir string, args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
