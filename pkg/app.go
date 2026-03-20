package pkg

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const defaultMaxWorkers = 3

// App is the Wails application struct. Methods on App are exposed to the Vue frontend.
type App struct {
	ctx           context.Context
	workDir       string
	mu            sync.Mutex                 // guards workDir string access only
	repo          Repository                 // database repository
	designMu      sync.Mutex                 // serializes writes to design document
	activeTasksMu sync.Mutex                 // guards activeTasks map
	activeTasks   map[int]context.CancelFunc // prevents double-pickup of tasks
	maxWorkers    int                        // max concurrent agent tasks
	connected     bool                       // whether the database is connected
	chatLocksMu   sync.Mutex                 // guards chatLocks map
	chatLocks     map[int]bool               // prevents overlapping Claude chat invocations per task
	loopCancel    context.CancelFunc         // cancels the current background loop
}

func NewApp() *App {
	dir, _ := os.Getwd()
	return &App{
		workDir:     dir,
		activeTasks: make(map[int]context.CancelFunc),
		maxWorkers:  defaultMaxWorkers,
		chatLocks:   make(map[int]bool),
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Try to connect using env var first, then config file.
	dbURL := os.Getenv("TWIST_DATABASE_URL")
	if dbURL == "" {
		if cfg, err := LoadConfig(); err == nil && cfg.DatabaseURL != "" {
			dbURL = cfg.DatabaseURL
		}
	}

	if dbURL != "" {
		if err := a.connectDB(dbURL); err != nil {
			a.log(fmt.Sprintf("[ERROR] Failed to connect to database: %v", err))
		}
	}

	// Emit initial state so frontend knows whether we're connected.
	a.emitDBStatus()
}

// ── Database Connection (exposed to Vue) ──────────────────────────────────────

// DBStatus represents the current database connection state.
type DBStatus struct {
	Connected   bool   `json:"connected"`
	DatabaseURL string `json:"database_url"`
}

// GetDBStatus returns the current database connection status.
func (a *App) GetDBStatus() DBStatus {
	url := ""
	if cfg, err := LoadConfig(); err == nil {
		url = cfg.DatabaseURL
	}
	if envURL := os.Getenv("TWIST_DATABASE_URL"); envURL != "" {
		url = envURL
	}
	return DBStatus{
		Connected:   a.connected,
		DatabaseURL: url,
	}
}

// ConnectDB connects to a PostgreSQL database, pings it, saves the URL to config,
// and starts the background loop. This is called from the frontend.
func (a *App) ConnectDB(databaseURL string) error {
	if databaseURL == "" {
		return fmt.Errorf("database URL is required")
	}

	if err := a.connectDB(databaseURL); err != nil {
		return err
	}

	// Save URL to config for future startups.
	if err := SaveConfig(&Config{DatabaseURL: databaseURL}); err != nil {
		a.log(fmt.Sprintf("[WARN] Could not save config: %v", err))
	}

	return nil
}

// connectDB is the internal connect + initialize method.
func (a *App) connectDB(databaseURL string) error {
	repo, err := NewPostgresRepository(databaseURL)
	if err != nil {
		return err
	}

	// Close previous connection if any.
	if a.repo != nil {
		a.repo.Close()
	}

	a.log("[DB] Connected to PostgreSQL")

	// Run file-based migrations. Progress is emitted to the frontend.
	result, err := RunMigrations(databaseURL, func(status MigrationStatus) {
		a.log(fmt.Sprintf("[MIGRATE] %s", status.Description))
		a.emitMigrationStatus(status)
	})
	if err != nil {
		repo.Close()
		return fmt.Errorf("migration failed: %w", err)
	}
	if result.Applied > 0 {
		a.log(fmt.Sprintf("[MIGRATE] %d migration(s) applied (total: %d)", result.Applied, result.Total))
	}

	a.repo = repo
	a.connected = true

	// Load persisted working directory.
	if saved, err := repo.GetSetting("workDir"); err == nil && saved != "" {
		a.mu.Lock()
		a.workDir = saved
		a.mu.Unlock()
	}

	// Load max workers setting.
	if val, err := repo.GetSetting("maxWorkers"); err == nil && val != "" {
		if n, err := strconv.Atoi(val); err == nil && n >= 1 && n <= 10 {
			a.maxWorkers = n
		}
	}

	// Clean up orphan worktrees from previous runs.
	cleanOrphanWorktrees(a.workDir, repo, a.log)

	a.emitTasks()
	a.emitDBStatus()

	// Cancel previous background loop before starting a new one.
	if a.loopCancel != nil {
		a.loopCancel()
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	a.loopCancel = cancel
	go a.runLoop(loopCtx)

	return nil
}

func (a *App) emitDBStatus() {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "db:status", a.GetDBStatus())
}

func (a *App) emitMigrationStatus(status MigrationStatus) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "migration:status", status)
}

// ── Exposed to Vue ────────────────────────────────────────────────────────────

// LoadTasks returns all tasks from the database.
func (a *App) LoadTasks() []Task {
	if a.repo == nil {
		return []Task{}
	}
	tasks, err := a.repo.LoadTasks()
	if err != nil {
		a.log(fmt.Sprintf("[ERROR] %v", err))
		return []Task{}
	}
	if tasks == nil {
		return []Task{}
	}
	return tasks
}

// AddTask creates a new task in the prompt lane (not yet approved).
func (a *App) AddTask(title, prompt string) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}
	_, err := a.repo.InsertTask(Task{
		Title:    title,
		Prompt:   prompt,
		Status:   "prompt",
		Approved: false,
	})
	if err != nil {
		return err
	}
	a.emitTasks()
	return nil
}

// AddTaskWithSpec creates a new task directly in the spec lane with a user-supplied spec.
func (a *App) AddTaskWithSpec(title, spec string) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(spec) == "" {
		return fmt.Errorf("title and spec are required")
	}
	id, err := a.repo.InsertTask(Task{
		Title:    title,
		Spec:     spec,
		Status:   "spec",
		Approved: false,
	})
	if err != nil {
		return err
	}

	// Append spec summary to design document for cross-task context.
	summary := fmt.Sprintf("Task #%d spec: %s", id, title)
	section := fmt.Sprintf("## Task #%d: %s (Spec)\n\n%s\n", id, title, truncate(spec, 500))
	appendDesignVersion(a.repo, &a.designMu, a.workDir, int(id), section, summary)

	a.emitTasks()
	return nil
}

// ApproveTask sets approved=true on a task and advances status where needed.
// spec → sets status to "code" and approved to true
// review → sets status to "done" (terminal) and approved to false
// prompt/code → just sets approved to true
func (a *App) ApproveTask(id int) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}

	task, err := a.repo.GetTaskByID(id)
	if err != nil {
		return fmt.Errorf("task #%d not found: %w", id, err)
	}

	if task.Status == "archived" {
		return fmt.Errorf("cannot approve archived task %d — restore it first", id)
	}

	newStatus := task.Status
	approved := true
	switch task.Status {
	case "spec":
		newStatus = "code"
	case "review":
		newStatus = "done"
		approved = false // done is terminal, no agent action needed
	}

	if err := a.repo.UpdateTaskStatus(id, newStatus, approved); err != nil {
		return err
	}

	// Record approval events.
	switch task.Status {
	case "spec":
		_ = a.repo.InsertTaskEvent(id, "spec_approved", "user", "Spec approved — moved to code", "")
	case "review":
		_ = a.repo.InsertTaskEvent(id, "review_approved", "user", "Review approved — moved to done", "")
	default:
		if newStatus != task.Status {
			_ = a.repo.InsertTaskEvent(id, "status_change", "system",
				fmt.Sprintf("Status changed to %s", newStatus),
				fmt.Sprintf("%s → %s", task.Status, newStatus))
		}
	}

	a.emitTasks()
	return nil
}

// DeleteTask removes a task by ID, cleaning up its worktree if present.
func (a *App) DeleteTask(id int) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}

	// Clean up worktree if the task has one.
	task, err := a.repo.GetTaskByID(id)
	if err == nil && task.WorktreePath != "" {
		a.mu.Lock()
		dir := a.workDir
		a.mu.Unlock()
		if err := removeWorktree(dir, task.WorktreePath); err != nil {
			a.log(fmt.Sprintf("[WARN] Failed to remove worktree for task #%d: %v", id, err))
		}
	}

	// Cancel active processing if running.
	a.activeTasksMu.Lock()
	if cancel, ok := a.activeTasks[id]; ok {
		cancel()
		delete(a.activeTasks, id)
	}
	a.activeTasksMu.Unlock()

	if err := a.repo.DeleteTask(id); err != nil {
		return err
	}
	a.emitTasks()
	return nil
}

// UpdateTask allows editing title, prompt, and spec of tasks in prompt or spec lanes.
// Edits are blocked while the agent is processing (approved = true).
// If the prompt changes while in the prompt lane, session IDs are cleared for a fresh spec generation.
func (a *App) UpdateTask(id int, title, prompt, spec string) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}

	task, err := a.repo.GetTaskByID(id)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	if task.Status != "prompt" && task.Status != "spec" {
		return fmt.Errorf("cannot edit task in %q status", task.Status)
	}

	if task.Approved {
		return fmt.Errorf("cannot edit task while it is being processed")
	}

	if err := a.repo.UpdateTaskFields(id, title, prompt, spec); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	// If prompt changed while in prompt status, clear sessions for fresh spec generation.
	if task.Status == "prompt" && strings.TrimSpace(prompt) != strings.TrimSpace(task.Prompt) {
		_ = a.repo.UpdateTaskSessionID(id, "")
		_ = a.repo.UpdateTaskChatSessionID(id, "")
	}

	a.emitTasks()
	return nil
}

// ArchiveTask moves a task to the archived status.
func (a *App) ArchiveTask(id int) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}
	if err := a.repo.ArchiveTask(id); err != nil {
		return fmt.Errorf("archive task %d: %w", id, err)
	}
	a.emitTasks()
	return nil
}

// RestoreTask moves an archived task back to prompt, clearing agent fields.
func (a *App) RestoreTask(id int) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}
	if err := a.repo.RestoreTask(id); err != nil {
		return fmt.Errorf("restore task %d: %w", id, err)
	}
	a.emitTasks()
	return nil
}

// GetWorkDir returns the current working directory.
func (a *App) GetWorkDir() string {
	return a.workDir
}

// PickDirectory opens the native OS directory picker and returns the chosen path
// without changing any application state. The caller decides whether to apply it.
func (a *App) PickDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Select Project Directory",
		DefaultDirectory:     a.workDir,
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// SetWorkDir changes the working directory to the provided path.
func (a *App) SetWorkDir(path string) error {
	if path == "" {
		return nil
	}
	return a.changeWorkDir(path)
}

// changeWorkDir switches the app to a new working directory.
func (a *App) changeWorkDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.workDir = abs
	a.mu.Unlock()

	// Persist the setting.
	if a.repo != nil {
		_ = a.repo.SetSetting("workDir", abs)
	}

	a.emitTasks()
	a.log(fmt.Sprintf("[CONFIG] Working directory set to: %s", abs))
	return nil
}

// GetSettings returns all user-configurable settings as a map.
func (a *App) GetSettings() (map[string]string, error) {
	dbURL := ""
	if envURL := os.Getenv("TWIST_DATABASE_URL"); envURL != "" {
		dbURL = envURL
	} else if cfg, err := LoadConfig(); err == nil {
		dbURL = cfg.DatabaseURL
	}

	result := map[string]string{
		"workDir":     a.workDir,
		"maxWorkers":  strconv.Itoa(a.maxWorkers),
		"databaseURL": dbURL,
	}
	return result, nil
}

// SaveSettings persists the provided key/value pairs and applies any settings
// that require runtime side-effects (e.g., workDir change).
func (a *App) SaveSettings(settings map[string]string) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}

	oldWorkDir := a.workDir

	// Persist each setting.
	for key, value := range settings {
		if err := a.repo.SetSetting(key, value); err != nil {
			return err
		}
	}

	// Apply workDir change if it differs from the current value.
	if newDir, ok := settings["workDir"]; ok && newDir != "" && newDir != oldWorkDir {
		if err := a.changeWorkDir(newDir); err != nil {
			return err
		}
	}

	// Apply maxWorkers change.
	if val, ok := settings["maxWorkers"]; ok && val != "" {
		if n, err := strconv.Atoi(val); err == nil && n >= 1 && n <= 10 {
			a.maxWorkers = n
		}
	}

	return nil
}

// ── Chat API (exposed to Vue) ──────────────────────────────────────────────────

// GetChatTimeline returns a merged timeline of workflow events and chat messages for a task.
// It triggers backfill for pre-existing tasks on first call.
func (a *App) GetChatTimeline(taskID int) []ChatTimelineEntry {
	if a.repo == nil {
		return []ChatTimelineEntry{}
	}
	pgRepo, ok := a.repo.(*PostgresRepository)
	if !ok {
		return []ChatTimelineEntry{}
	}
	// Lazily backfill events for pre-existing tasks.
	if err := pgRepo.BackfillTaskEvents(taskID); err != nil {
		a.log(fmt.Sprintf("[WARN] BackfillTaskEvents: %v", err))
	}
	timeline, err := pgRepo.GetChatTimeline(taskID)
	if err != nil {
		a.log(fmt.Sprintf("[ERROR] GetChatTimeline: %v", err))
		return []ChatTimelineEntry{}
	}
	if timeline == nil {
		return []ChatTimelineEntry{}
	}
	return timeline
}

// GetChatMessages returns stored chat history for a task.
func (a *App) GetChatMessages(taskID int) []ChatMessage {
	if a.repo == nil {
		return []ChatMessage{}
	}
	msgs, err := a.repo.GetChatMessages(taskID)
	if err != nil {
		a.log(fmt.Sprintf("[ERROR] GetChatMessages: %v", err))
		return []ChatMessage{}
	}
	if msgs == nil {
		return []ChatMessage{}
	}
	return msgs
}

// chatInvokeOpts configures a Claude CLI chat invocation.
type chatInvokeOpts struct {
	ResumeSessionID string // use --resume <id>
	Fork            bool   // add --fork-session flag (boolean, modifies --resume)
	SessionID       string // use --session-id <id> (fallback, mutually exclusive with Resume)
	Message         string
	Dir             string
}

// chatInvokeResult holds the response and any extracted session ID from stream-json output.
type chatInvokeResult struct {
	Response  string
	SessionID string // extracted from stream-json; empty if not found
}

// streamLine represents a single JSON line from Claude's --output-format stream-json output.
type streamLine struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Content   string `json:"content,omitempty"`
	Result    string `json:"result,omitempty"`
}

// parseStreamLine parses a single line of stream-json output, returning
// the session ID, content text, result text, and whether parsing succeeded.
func parseStreamLine(line string) (sessionID, content, result string, ok bool) {
	var sl streamLine
	if err := json.Unmarshal([]byte(line), &sl); err != nil {
		return "", "", "", false
	}
	return sl.SessionID, sl.Content, sl.Result, true
}

// buildChatArgs constructs the Claude CLI argument list for a chat invocation.
func buildChatArgs(opts chatInvokeOpts) []string {
	args := []string{"-p", "--dangerously-skip-permissions", "--verbose", "--output-format", "stream-json"}
	if opts.ResumeSessionID != "" {
		args = append(args, "--resume", opts.ResumeSessionID)
		if opts.Fork {
			args = append(args, "--fork-session")
		}
	} else if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}
	args = append(args, opts.Message)
	return args
}

// SendChatMessage accepts a user message, invokes Claude with a dedicated chat session, and streams the response back.
// Uses --resume --fork-session to fork from the workflow session, giving the chat full conversation history.
// Falls back to context injection when no workflow session exists or when the session is locked.
func (a *App) SendChatMessage(taskID int, message string) error {
	if a.repo == nil {
		return fmt.Errorf("database not connected")
	}

	// Acquire per-task chat lock.
	a.chatLocksMu.Lock()
	if a.chatLocks[taskID] {
		a.chatLocksMu.Unlock()
		return fmt.Errorf("chat already in progress for task #%d", taskID)
	}
	a.chatLocks[taskID] = true
	a.chatLocksMu.Unlock()

	// Look up the task.
	task, err := a.repo.GetTaskByID(taskID)
	if err != nil {
		a.chatLocksMu.Lock()
		delete(a.chatLocks, taskID)
		a.chatLocksMu.Unlock()
		return fmt.Errorf("task #%d not found: %w", taskID, err)
	}

	// Insert the user message.
	userMsg, err := a.repo.InsertChatMessage(taskID, "user", message)
	if err != nil {
		a.chatLocksMu.Lock()
		delete(a.chatLocks, taskID)
		a.chatLocksMu.Unlock()
		return fmt.Errorf("failed to save message: %w", err)
	}

	// Emit user message to frontend.
	runtime.EventsEmit(a.ctx, "chat:message", userMsg)

	// Determine working directory.
	a.mu.Lock()
	dir := a.workDir
	a.mu.Unlock()
	if task.WorktreePath != "" {
		if _, err := os.Stat(task.WorktreePath); err == nil {
			dir = task.WorktreePath
		}
	}

	// Run Claude in a goroutine to avoid blocking.
	go func() {
		defer func() {
			a.chatLocksMu.Lock()
			delete(a.chatLocks, taskID)
			a.chatLocksMu.Unlock()
		}()

		var result chatInvokeResult
		var invokeErr error

		if task.ChatSessionID != "" {
			// Case A: resume existing chat session (no fork needed).
			a.log(fmt.Sprintf("[CHAT] Task #%d: resuming chat session %s", taskID, task.ChatSessionID))
			result, invokeErr = a.invokeChatClaude(taskID, chatInvokeOpts{
				ResumeSessionID: task.ChatSessionID,
				Message:         message,
				Dir:             dir,
			})
			if invokeErr != nil && strings.Contains(invokeErr.Error(), "already in use") {
				// Resumed chat session conflict — generate new UUID, retry with --session-id + context injection.
				a.log(fmt.Sprintf("[CHAT] Task #%d: chat session conflict, falling back to new session", taskID))
				newSessionID := generateUUID()
				if dbErr := a.repo.UpdateTaskChatSessionID(taskID, newSessionID); dbErr != nil {
					runtime.EventsEmit(a.ctx, "chat:error", map[string]interface{}{"task_id": taskID, "error": dbErr.Error()})
					return
				}
				contextMsg := buildChatContextMessage(task.Title, task.Spec, message)
				result, invokeErr = a.invokeChatClaude(taskID, chatInvokeOpts{
					SessionID: newSessionID,
					Message:   contextMsg,
					Dir:       dir,
				})
				if invokeErr == nil {
					chatID := newSessionID
					if result.SessionID != "" {
						chatID = result.SessionID
					}
					_ = a.repo.UpdateTaskChatSessionID(taskID, chatID)
				}
			}
		} else if task.SessionID != "" {
			// Case B: fork from workflow session.
			a.log(fmt.Sprintf("[CHAT] Task #%d: forking from workflow session %s", taskID, task.SessionID))
			result, invokeErr = a.invokeChatClaude(taskID, chatInvokeOpts{
				ResumeSessionID: task.SessionID,
				Fork:            true,
				Message:         message,
				Dir:             dir,
			})
			if invokeErr != nil && strings.Contains(invokeErr.Error(), "already in use") {
				// Workflow session locked — wait 2s, retry once.
				a.log(fmt.Sprintf("[CHAT] Task #%d: workflow session locked, retrying in 2s", taskID))
				time.Sleep(2 * time.Second)
				result, invokeErr = a.invokeChatClaude(taskID, chatInvokeOpts{
					ResumeSessionID: task.SessionID,
					Fork:            true,
					Message:         message,
					Dir:             dir,
				})
				if invokeErr != nil {
					// Fall back to fresh session + context injection.
					a.log(fmt.Sprintf("[CHAT] Task #%d: fork retry failed, falling back to context injection", taskID))
					newSessionID := generateUUID()
					contextMsg := buildChatContextMessage(task.Title, task.Spec, message)
					result, invokeErr = a.invokeChatClaude(taskID, chatInvokeOpts{
						SessionID: newSessionID,
						Message:   contextMsg,
						Dir:       dir,
					})
					if invokeErr == nil {
						chatID := newSessionID
						if result.SessionID != "" {
							chatID = result.SessionID
						}
						_ = a.repo.UpdateTaskChatSessionID(taskID, chatID)
					}
				}
			}
			if invokeErr == nil && result.SessionID != "" {
				// Store forked session ID for subsequent --resume calls.
				_ = a.repo.UpdateTaskChatSessionID(taskID, result.SessionID)
			}
		} else {
			// Case C: no workflow session — fresh session with context injection.
			a.log(fmt.Sprintf("[CHAT] Task #%d: no workflow session, using fresh session with context", taskID))
			newSessionID := generateUUID()
			contextMsg := buildChatContextMessage(task.Title, task.Spec, message)
			result, invokeErr = a.invokeChatClaude(taskID, chatInvokeOpts{
				SessionID: newSessionID,
				Message:   contextMsg,
				Dir:       dir,
			})
			if invokeErr == nil {
				chatID := newSessionID
				if result.SessionID != "" {
					chatID = result.SessionID
				}
				_ = a.repo.UpdateTaskChatSessionID(taskID, chatID)
			}
		}

		if invokeErr != nil {
			runtime.EventsEmit(a.ctx, "chat:error", map[string]interface{}{"task_id": taskID, "error": invokeErr.Error()})
			return
		}

		// Save assistant response.
		if result.Response != "" {
			a.repo.InsertChatMessage(taskID, "assistant", result.Response)
		}

		runtime.EventsEmit(a.ctx, "chat:done", map[string]interface{}{"task_id": taskID})
	}()

	return nil
}

// invokeChatClaude runs the Claude CLI with the given options, parsing stream-json output.
// Returns the response text and any session ID extracted from the stream.
// If stderr contains "already in use", the error message will contain that substring.
func (a *App) invokeChatClaude(taskID int, opts chatInvokeOpts) (chatInvokeResult, error) {
	args := buildChatArgs(opts)
	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.Dir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return chatInvokeResult{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return chatInvokeResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return chatInvokeResult{}, err
	}

	// Drain stderr in background, capturing it for conflict detection.
	var stderrBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n")
			a.log(fmt.Sprintf("[CHAT ERR] Task #%d: %s", taskID, line))
		}
	}()

	// Parse stream-json output line-by-line.
	var result chatInvokeResult
	var fullResponse strings.Builder
	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()

		sessionID, content, resultText, ok := parseStreamLine(line)
		if !ok {
			// Treat as raw text (defensive fallback).
			fullResponse.WriteString(line + "\n")
			runtime.EventsEmit(a.ctx, "chat:stream", map[string]interface{}{"task_id": taskID, "chunk": line + "\n"})
			continue
		}

		if sessionID != "" && result.SessionID == "" {
			result.SessionID = sessionID
		}
		if content != "" {
			fullResponse.WriteString(content)
			runtime.EventsEmit(a.ctx, "chat:stream", map[string]interface{}{"task_id": taskID, "chunk": content})
		}
		if resultText != "" {
			result.Response = resultText
		}
	}

	wg.Wait()
	if err := cmd.Wait(); err != nil {
		stderrStr := stderrBuf.String()
		if strings.Contains(stderrStr, "already in use") {
			return chatInvokeResult{}, fmt.Errorf("session already in use")
		}
		return chatInvokeResult{}, fmt.Errorf("claude exited with error: %v (stderr: %s)", err, stderrStr)
	}

	// If no explicit result was captured from a "result" message, use the accumulated response.
	if result.Response == "" {
		result.Response = strings.TrimSpace(fullResponse.String())
	}

	return result, nil
}

// buildChatContextMessage wraps the user's message with task context for a new chat session.
func buildChatContextMessage(title, spec, userMessage string) string {
	return fmt.Sprintf(`You are assisting with a software task.

Task: %s

Specification:
%s

The user wants to discuss this task with you. Respond to their message below.

---

%s`, title, spec, userMessage)
}

// ── Background Loop ───────────────────────────────────────────────────────────

func (a *App) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		if a.repo == nil || !a.connected {
			continue
		}

		a.mu.Lock()
		dir := a.workDir
		a.mu.Unlock()

		tasks, err := a.repo.FindActionableTasks()
		if err != nil {
			a.log(fmt.Sprintf("[ERROR] %v", err))
			continue
		}

		for _, task := range tasks {
			a.activeTasksMu.Lock()
			// Skip if already running.
			if _, running := a.activeTasks[task.ID]; running {
				a.activeTasksMu.Unlock()
				continue
			}
			// Skip if at capacity.
			if len(a.activeTasks) >= a.maxWorkers {
				a.activeTasksMu.Unlock()
				break
			}
			taskCtx, cancel := context.WithCancel(context.Background())
			a.activeTasks[task.ID] = cancel
			a.activeTasksMu.Unlock()

			go a.processTask(taskCtx, task, dir)
		}
	}
}

func (a *App) processTask(ctx context.Context, task Task, dir string) {
	defer func() {
		a.activeTasksMu.Lock()
		delete(a.activeTasks, task.ID)
		a.activeTasksMu.Unlock()
		a.emitTasks()
		a.emitActiveCount()
	}()

	a.emitActiveCount()

	var handlerErr error
	switch task.Status {
	case "prompt":
		handlerErr = handlePrompt(task, dir, a.repo, &a.designMu, a.log)
	case "code":
		handlerErr = handleCode(task, dir, a.repo, &a.designMu, a.log)
	}

	if handlerErr != nil {
		a.log(fmt.Sprintf("[ERROR] Task #%d: %v", task.ID, handlerErr))
	}
}

// ── Design Document API (exposed to Vue) ──────────────────────────────────────

// GetDesignDoc returns the current design document content.
func (a *App) GetDesignDoc() string {
	a.mu.Lock()
	dir := a.workDir
	a.mu.Unlock()
	return readDesignContent(dir)
}

// GetDesignHistory returns all design document versions.
func (a *App) GetDesignHistory() []DesignVersion {
	if a.repo == nil {
		return []DesignVersion{}
	}
	versions, err := a.repo.GetDesignHistory()
	if err != nil {
		return []DesignVersion{}
	}
	if versions == nil {
		return []DesignVersion{}
	}
	return versions
}

// GetActiveCount returns how many tasks are currently being processed.
func (a *App) GetActiveCount() int {
	a.activeTasksMu.Lock()
	defer a.activeTasksMu.Unlock()
	return len(a.activeTasks)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (a *App) emitTasks() {
	if a.ctx == nil {
		return
	}
	var tasks []Task
	if a.repo != nil {
		var err error
		tasks, err = a.repo.LoadTasks()
		if err != nil {
			tasks = []Task{}
		}
	}
	if tasks == nil {
		tasks = []Task{}
	}
	runtime.EventsEmit(a.ctx, "tasks:updated", tasks)
}

func (a *App) emitActiveCount() {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "activeCount:updated", a.GetActiveCount())
}

func (a *App) log(msg string) {
	fmt.Println(msg)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", msg)
	}
}
