package pkg

import (
	"bufio"
	"crypto/rand"
	"database/sql"
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
// Status flow: prompt → spec → code → review → done
//
//	(agent)       (user)  (agent+PR)  (user) (terminal)
//
// The agent only acts when Approved is true. After each agent action it resets
// Approved to false so the user must explicitly re-approve the next stage.
type Task struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Prompt       string `json:"prompt"`        // user writes this
	Spec         string `json:"spec"`          // agent writes this
	Branch       string `json:"branch"`        // agent sets this
	PRURL        string `json:"pr_url"`        // agent sets this after code phase
	Status       string `json:"status"`        // prompt | spec | code | review | done | failed
	Approved     bool   `json:"approved"`      // user sets true to let agent act; agent resets to false after each stage
	SessionID    string `json:"session_id"`    // claude session ID for continuity across phases
	WorktreePath string `json:"worktree_path"` // git worktree path for code isolation
}

// DesignVersion represents one immutable snapshot of the shared design document.
type DesignVersion struct {
	ID        int    `json:"id"`
	Version   int    `json:"version"`
	Content   string `json:"content"`
	TaskID    int    `json:"task_id"`
	CreatedAt string `json:"created_at"`
	Summary   string `json:"summary"`
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
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    title         TEXT    NOT NULL DEFAULT '',
    prompt        TEXT    NOT NULL DEFAULT '',
    spec          TEXT    NOT NULL DEFAULT '',
    branch        TEXT    NOT NULL DEFAULT '',
    pr_url        TEXT    NOT NULL DEFAULT '',
    status        TEXT    NOT NULL DEFAULT 'prompt',
    approved      INTEGER NOT NULL DEFAULT 0,
    session_id    TEXT    NOT NULL DEFAULT '',
    worktree_path TEXT    NOT NULL DEFAULT ''
);`

const settingsSchema = `
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);`

const designVersionsSchema = `
CREATE TABLE IF NOT EXISTS design_versions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    version    INTEGER NOT NULL,
    content    TEXT    NOT NULL DEFAULT '',
    task_id    INTEGER NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    summary    TEXT    NOT NULL DEFAULT ''
);`

func OpenDB(dir string) (*sql.DB, error) {
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
	if _, err := db.Exec(settingsSchema); err != nil {
		return err
	}
	if _, err := db.Exec(designVersionsSchema); err != nil {
		return err
	}
	// Migrations for existing databases that lack newer columns.
	db.Exec(`ALTER TABLE tasks ADD COLUMN pr_url TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE tasks ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE tasks ADD COLUMN worktree_path TEXT NOT NULL DEFAULT ''`)
	return nil
}

func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?,?)`, key, value)
	return err
}

func scanTask(row *sql.Row) (Task, error) {
	var t Task
	var approved int
	err := row.Scan(&t.ID, &t.Title, &t.Prompt, &t.Spec, &t.Branch, &t.PRURL, &t.Status, &approved, &t.SessionID, &t.WorktreePath)
	t.Approved = approved == 1
	return t, err
}

const taskColumns = `id, title, prompt, spec, branch, pr_url, status, approved, session_id, worktree_path`

func loadTasks(db *sql.DB) ([]Task, error) {
	rows, err := db.Query(`SELECT ` + taskColumns + ` FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		var approved int
		if err := rows.Scan(&t.ID, &t.Title, &t.Prompt, &t.Spec, &t.Branch, &t.PRURL, &t.Status, &approved, &t.SessionID, &t.WorktreePath); err != nil {
			return nil, err
		}
		t.Approved = approved == 1
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func insertTask(db *sql.DB, t Task) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO tasks (title, prompt, spec, branch, pr_url, status, approved, session_id, worktree_path) VALUES (?,?,?,?,?,?,?,?,?)`,
		t.Title, t.Prompt, t.Spec, t.Branch, t.PRURL, t.Status, boolToInt(t.Approved), t.SessionID, t.WorktreePath,
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

func updateTaskPRURL(db *sql.DB, id int, prURL string) error {
	_, err := db.Exec(`UPDATE tasks SET pr_url=? WHERE id=?`, prURL, id)
	return err
}

func updateTaskSessionID(db *sql.DB, id int, sessionID string) error {
	_, err := db.Exec(`UPDATE tasks SET session_id=? WHERE id=?`, sessionID, id)
	return err
}

func updateTaskWorktreePath(db *sql.DB, id int, path string) error {
	_, err := db.Exec(`UPDATE tasks SET worktree_path=? WHERE id=?`, path, id)
	return err
}

func deleteTask(db *sql.DB, id int) error {
	_, err := db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}

func getTaskByID(db *sql.DB, id int) (Task, error) {
	row := db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id=?`, id)
	return scanTask(row)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func findActionableDB(db *sql.DB) (Task, bool, error) {
	row := db.QueryRow(
		`SELECT `+taskColumns+` FROM tasks
         WHERE approved=1 AND status IN ('prompt','code')
         ORDER BY id LIMIT 1`,
	)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	return t, err == nil, err
}

func findActionableTasksDB(db *sql.DB) ([]Task, error) {
	rows, err := db.Query(
		`SELECT `+taskColumns+` FROM tasks
         WHERE approved=1 AND status IN ('prompt','code')
         ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		var approved int
		if err := rows.Scan(&t.ID, &t.Title, &t.Prompt, &t.Spec, &t.Branch, &t.PRURL, &t.Status, &approved, &t.SessionID, &t.WorktreePath); err != nil {
			return nil, err
		}
		t.Approved = approved == 1
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ── Lane Handlers ─────────────────────────────────────────────────────────────

// handlePrompt: reads the user's prompt, writes a spec, moves task to 'spec' lane.
func handlePrompt(task Task, workDir string, db *sql.DB, designMu *sync.Mutex, log LogFunc) error {
	log(fmt.Sprintf("[SPEC] Task #%d: Writing spec for — %s", task.ID, task.Title))

	// Generate a session ID for this task so claude remembers the conversation.
	sessionID := generateUUID()
	if err := updateTaskSessionID(db, task.ID, sessionID); err != nil {
		return err
	}

	// Read latest design doc + cross-task context for the prompt.
	designContent := readDesignContent(workDir)
	crossCtx, _ := buildTaskContext(db, task.ID)

	var contextBlock string
	if designContent != "" {
		contextBlock += fmt.Sprintf("\n\n--- Current Design Document ---\n%s\n--- End Design Document ---\n", designContent)
	}
	if crossCtx != "" {
		contextBlock += fmt.Sprintf("\n\n--- Other Task Specs ---\n%s\n--- End Other Task Specs ---\n", crossCtx)
	}

	prompt := fmt.Sprintf(`You are a software architect. A user has the following request:

%s
%s
Write a detailed technical specification as plain markdown. Include:
- Overview and goals
- Technical requirements
- File/directory structure
- Implementation steps
- Acceptance criteria

Return ONLY the markdown spec, no other commentary.`, task.Prompt, contextBlock)

	spec, err := runClaudeCapture(task.ID, prompt, workDir, sessionID, log)
	if err != nil {
		return fmt.Errorf("spec generation failed for task #%d: %v", task.ID, err)
	}

	trimmedSpec := strings.TrimSpace(spec)
	if err := updateTaskSpec(db, task.ID, trimmedSpec); err != nil {
		return err
	}
	if err := updateTaskStatus(db, task.ID, "spec", false); err != nil {
		return err
	}

	// Append spec summary to design document.
	summary := fmt.Sprintf("Task #%d spec: %s", task.ID, task.Title)
	section := fmt.Sprintf("## Task #%d: %s (Spec)\n\n%s\n", task.ID, task.Title, truncate(trimmedSpec, 500))
	appendDesignVersion(db, designMu, workDir, task.ID, section, summary)

	log(fmt.Sprintf("[SPEC READY] Task #%d: Spec written. Set status → 'code' + approved → true to approve.", task.ID))
	return nil
}

// handleCode: creates a git worktree, runs claude to implement and commit, moves task to 'review' lane.
func handleCode(task Task, workDir string, db *sql.DB, designMu *sync.Mutex, log LogFunc) error {
	log(fmt.Sprintf("[CODE] Task #%d: Implementing — %s", task.ID, task.Title))

	branch := fmt.Sprintf("feature/task-%d-%s", task.ID, slugify(task.Title))

	// Create an isolated worktree for this task.
	wtPath, err := createWorktree(workDir, branch, task.ID)
	if err != nil {
		return fmt.Errorf("worktree creation failed for task #%d: %v", task.ID, err)
	}
	log(fmt.Sprintf("[GIT] Created worktree: %s (branch: %s)", wtPath, branch))

	if err := updateTaskBranch(db, task.ID, branch); err != nil {
		return err
	}
	if err := updateTaskWorktreePath(db, task.ID, wtPath); err != nil {
		return err
	}

	// Copy DESIGN.md into the worktree so claude can read it.
	designContent := readDesignContent(workDir)
	if designContent != "" {
		_ = os.WriteFile(filepath.Join(wtPath, "DESIGN.md"), []byte(designContent), 0644)
	}

	prompt := fmt.Sprintf(`Implement the following specification exactly as written.
Run tests after implementation. Commit all changes with a descriptive commit message.

Specification:
%s

The git branch '%s' is already checked out. Do NOT create a new branch.
Do not ask for permission — implement, test, and commit.`, task.Spec, branch)

	if err := runClaudeStream(task.ID, prompt, wtPath, task.SessionID, log); err != nil {
		log(fmt.Sprintf("[FAILED] Task #%d implementation error: %v", task.ID, err))
		log(fmt.Sprintf("         Worktree preserved at: %s", wtPath))
		log("         Fix the issue manually, then set status → 'code' + approved → true to retry.")
		_ = updateTaskStatus(db, task.ID, "failed", false)
		return nil
	}

	// Push branch and create PR automatically after successful implementation.
	log(fmt.Sprintf("[GIT] Pushing branch: %s", branch))
	if out, err := gitCmd(wtPath, "push", "-u", "origin", branch); err != nil {
		log(fmt.Sprintf("[FAILED] Task #%d push error: %v\n%s", task.ID, err, out))
		_ = updateTaskStatus(db, task.ID, "failed", false)
		return nil
	}

	prBody := fmt.Sprintf("## Task #%d: %s\n\n### Spec\n\n%s\n\n---\n🤖 Raised by twist",
		task.ID, task.Title, task.Spec)

	prOut, err := ghCmd(wtPath, "pr", "create",
		"--title", fmt.Sprintf("Task #%d: %s", task.ID, task.Title),
		"--body", prBody,
	)
	if err != nil {
		log(fmt.Sprintf("[FAILED] Task #%d PR creation error: %v\n%s", task.ID, err, prOut))
		_ = updateTaskStatus(db, task.ID, "failed", false)
		return nil
	}

	prURL := strings.TrimSpace(prOut)
	log(fmt.Sprintf("[PR RAISED] %s", prURL))

	if err := updateTaskPRURL(db, task.ID, prURL); err != nil {
		return err
	}
	if err := updateTaskStatus(db, task.ID, "review", false); err != nil {
		return err
	}

	// Clean up worktree on success.
	if err := removeWorktree(workDir, wtPath); err != nil {
		log(fmt.Sprintf("[WARN] Task #%d: failed to remove worktree: %v", task.ID, err))
	}
	_ = updateTaskWorktreePath(db, task.ID, "")

	// Get files changed and append to design document.
	filesOut, _ := gitCmd(workDir, "diff", "--name-only", "HEAD", branch)
	summary := fmt.Sprintf("Task #%d code: %s", task.ID, task.Title)
	section := fmt.Sprintf("## Task #%d: %s (Code Complete)\n\nPR: %s\nFiles changed:\n%s\n",
		task.ID, task.Title, prURL, strings.TrimSpace(filesOut))
	appendDesignVersion(db, designMu, workDir, task.ID, section, summary)

	log(fmt.Sprintf("[REVIEW READY] Task #%d: PR created on branch '%s'. Review the PR and approve to move to done.", task.ID, branch))
	return nil
}

// ── Claude Helpers ────────────────────────────────────────────────────────────

func runClaudeCapture(taskID int, prompt string, workDir string, sessionID string, log LogFunc) (string, error) {
	args := []string{"-p", "--dangerously-skip-permissions"}
	if sessionID != "" {
		args = append(args, "--session-id", sessionID)
	}
	args = append(args, prompt)
	cmd := exec.Command("claude", args...)
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

func runClaudeStream(taskID int, prompt string, workDir string, sessionID string, log LogFunc) error {
	args := []string{"-p", "--dangerously-skip-permissions", "--verbose"}
	if sessionID != "" {
		args = append(args, "--session-id", sessionID)
	}
	args = append(args, prompt)
	cmd := exec.Command("claude", args...)
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

func generateUUID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ── Git Worktree Helpers ──────────────────────────────────────────────────────

func worktreeDir(workDir string) string {
	return filepath.Join(workDir, ".twist-worktrees")
}

func createWorktree(workDir, branch string, taskID int) (string, error) {
	dir := worktreeDir(workDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	wtPath := filepath.Join(dir, fmt.Sprintf("task-%d", taskID))

	// Remove stale worktree at same path if exists.
	_ = os.RemoveAll(wtPath)

	if _, err := gitCmd(workDir, "worktree", "add", wtPath, "-b", branch); err != nil {
		// Branch may already exist — try without -b.
		if _, err2 := gitCmd(workDir, "worktree", "add", wtPath, branch); err2 != nil {
			return "", fmt.Errorf("git worktree add failed: %v (also tried existing branch: %v)", err, err2)
		}
	}
	return wtPath, nil
}

func removeWorktree(workDir, wtPath string) error {
	if wtPath == "" {
		return nil
	}
	_, err := gitCmd(workDir, "worktree", "remove", wtPath, "--force")
	return err
}

// cleanOrphanWorktrees removes worktrees not linked to active tasks.
func cleanOrphanWorktrees(workDir string, db *sql.DB, log LogFunc) {
	dir := worktreeDir(workDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // directory doesn't exist yet, that's fine
	}

	// Collect active worktree paths.
	tasks, err := loadTasks(db)
	if err != nil {
		return
	}
	active := make(map[string]bool)
	for _, t := range tasks {
		if t.WorktreePath != "" {
			active[t.WorktreePath] = true
		}
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wtPath := filepath.Join(dir, e.Name())
		if !active[wtPath] {
			log(fmt.Sprintf("[CLEANUP] Removing orphan worktree: %s", wtPath))
			_ = removeWorktree(workDir, wtPath)
		}
	}

	// Prune any stale worktree entries in git.
	_, _ = gitCmd(workDir, "worktree", "prune")
}

// ── Design Document Helpers ───────────────────────────────────────────────────

func getLatestDesignVersion(db *sql.DB) (int, string, error) {
	var version int
	var content string
	err := db.QueryRow(`SELECT version, content FROM design_versions ORDER BY version DESC LIMIT 1`).Scan(&version, &content)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	return version, content, err
}

func appendDesignVersion(db *sql.DB, designMu *sync.Mutex, workDir string, taskID int, section, summary string) {
	designMu.Lock()
	defer designMu.Unlock()

	version, existing, _ := getLatestDesignVersion(db)
	newContent := existing
	if newContent != "" {
		newContent += "\n\n"
	}
	newContent += section
	newVersion := version + 1

	_, _ = db.Exec(
		`INSERT INTO design_versions (version, content, task_id, summary) VALUES (?,?,?,?)`,
		newVersion, newContent, taskID, summary,
	)

	// Write DESIGN.md to disk so agents can read it.
	_ = os.WriteFile(filepath.Join(workDir, "DESIGN.md"), []byte(newContent), 0644)
}

func getDesignHistory(db *sql.DB) ([]DesignVersion, error) {
	rows, err := db.Query(`SELECT id, version, content, task_id, created_at, summary FROM design_versions ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []DesignVersion
	for rows.Next() {
		var v DesignVersion
		if err := rows.Scan(&v.ID, &v.Version, &v.Content, &v.TaskID, &v.CreatedAt, &v.Summary); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func readDesignContent(workDir string) string {
	data, err := os.ReadFile(filepath.Join(workDir, "DESIGN.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

// buildTaskContext returns truncated specs from other tasks for cross-task awareness.
func buildTaskContext(db *sql.DB, excludeTaskID int) (string, error) {
	rows, err := db.Query(
		`SELECT id, title, spec FROM tasks WHERE spec != '' AND id != ? ORDER BY id DESC LIMIT 10`,
		excludeTaskID,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var parts []string
	for rows.Next() {
		var id int
		var title, spec string
		if err := rows.Scan(&id, &title, &spec); err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("### Task #%d: %s\n%s", id, title, truncate(spec, 500)))
	}
	return strings.Join(parts, "\n\n"), rows.Err()
}
