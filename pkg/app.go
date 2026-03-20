package pkg

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const defaultMaxWorkers = 3

// App is the Wails application struct. Methods on App are exposed to the Vue frontend.
type App struct {
	ctx           context.Context
	workDir       string
	mu            sync.Mutex // guards workDir string access only
	db            *sql.DB
	designMu      sync.Mutex                // serializes writes to design document
	activeTasksMu sync.Mutex                // guards activeTasks map
	activeTasks   map[int]context.CancelFunc // prevents double-pickup of tasks
	maxWorkers    int                        // max concurrent agent tasks
}

func NewApp() *App {
	dir, _ := os.Getwd()
	return &App{
		workDir:     dir,
		activeTasks: make(map[int]context.CancelFunc),
		maxWorkers:  defaultMaxWorkers,
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	db, err := OpenDB(a.workDir)
	if err != nil {
		a.log(fmt.Sprintf("[ERROR] Failed to open DB: %v", err))
		return
	}
	a.db = db

	// Load persisted working directory; if valid and different, switch to saved project DB.
	if saved, err := GetSetting(db, "workDir"); err == nil && saved != "" && saved != a.workDir {
		if newDB, err := OpenDB(saved); err == nil {
			db.Close()
			a.db = newDB
			a.workDir = saved
		}
	}

	// Load max workers setting.
	if val, err := GetSetting(a.db, "maxWorkers"); err == nil && val != "" {
		if n, err := strconv.Atoi(val); err == nil && n >= 1 && n <= 10 {
			a.maxWorkers = n
		}
	}

	// Clean up orphan worktrees from previous runs.
	cleanOrphanWorktrees(a.workDir, a.db, a.log)

	a.emitTasks()
	go a.runLoop()
}

// ── Exposed to Vue ────────────────────────────────────────────────────────────

// LoadTasks returns all tasks from the database.
func (a *App) LoadTasks() []Task {
	if a.db == nil {
		return []Task{}
	}
	tasks, err := loadTasks(a.db)
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
	if a.db == nil {
		return fmt.Errorf("database not initialised")
	}
	_, err := insertTask(a.db, Task{
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

// ApproveTask sets approved=true on a task and advances status where needed.
// spec → sets status to "code" and approved to true
// review → sets status to "done" (terminal) and approved to false
// prompt/code → just sets approved to true
func (a *App) ApproveTask(id int) error {
	if a.db == nil {
		return fmt.Errorf("database not initialised")
	}

	row := a.db.QueryRow(
		`SELECT `+taskColumns+` FROM tasks WHERE id=?`, id,
	)
	task, err := scanTask(row)
	if err == sql.ErrNoRows {
		return fmt.Errorf("task #%d not found", id)
	}
	if err != nil {
		return err
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

	if err := updateTaskStatus(a.db, id, newStatus, approved); err != nil {
		return err
	}
	a.emitTasks()
	return nil
}

// DeleteTask removes a task by ID, cleaning up its worktree if present.
func (a *App) DeleteTask(id int) error {
	if a.db == nil {
		return fmt.Errorf("database not initialised")
	}

	// Clean up worktree if the task has one.
	task, err := getTaskByID(a.db, id)
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

	if err := deleteTask(a.db, id); err != nil {
		return err
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
// It is also exposed as an IPC method for direct path setting.
func (a *App) SetWorkDir(path string) error {
	if path == "" {
		return nil
	}
	return a.changeWorkDir(path)
}

// changeWorkDir switches the app to a new working directory: updates a.workDir,
// reopens the database, and emits updated tasks.
func (a *App) changeWorkDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.workDir = abs
	a.mu.Unlock()

	if a.db != nil {
		a.db.Close()
	}
	db, err := OpenDB(abs)
	if err != nil {
		return err
	}
	a.db = db

	a.emitTasks()
	a.log(fmt.Sprintf("[CONFIG] Working directory set to: %s", abs))
	return nil
}

// GetSettings returns all user-configurable settings as a map.
func (a *App) GetSettings() (map[string]string, error) {
	result := map[string]string{
		"workDir":    a.workDir,
		"maxWorkers": strconv.Itoa(a.maxWorkers),
	}
	return result, nil
}

// SaveSettings persists the provided key/value pairs and applies any settings
// that require runtime side-effects (e.g., workDir change).
func (a *App) SaveSettings(settings map[string]string) error {
	if a.db == nil {
		return fmt.Errorf("database not initialised")
	}

	oldWorkDir := a.workDir

	// Persist each setting to the current DB before potentially switching.
	for key, value := range settings {
		if err := SetSetting(a.db, key, value); err != nil {
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

// ── Background Loop ───────────────────────────────────────────────────────────

func (a *App) runLoop() {
	for {
		time.Sleep(2 * time.Second)

		if a.db == nil {
			continue
		}

		a.mu.Lock()
		dir := a.workDir
		a.mu.Unlock()

		tasks, err := findActionableTasksDB(a.db)
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
			ctx, cancel := context.WithCancel(context.Background())
			a.activeTasks[task.ID] = cancel
			a.activeTasksMu.Unlock()

			go a.processTask(ctx, task, dir)
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
		handlerErr = handlePrompt(task, dir, a.db, &a.designMu, a.log)
	case "code":
		handlerErr = handleCode(task, dir, a.db, &a.designMu, a.log)
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
	if a.db == nil {
		return []DesignVersion{}
	}
	versions, err := getDesignHistory(a.db)
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
	if a.db != nil {
		var err error
		tasks, err = loadTasks(a.db)
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
