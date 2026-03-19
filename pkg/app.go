package pkg

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application struct. Methods on App are exposed to the Vue frontend.
type App struct {
	ctx     context.Context
	workDir string
	mu      sync.Mutex // guards workDir string access only
	db      *sql.DB
}

func NewApp() *App {
	dir, _ := os.Getwd()
	return &App{workDir: dir}
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

	migrateFromJSON(a.workDir, a.db)

	a.emitTasks()
	go a.runLoop()
}

// ── Exposed to Vue ────────────────────────────────────────────────────────────

// LoadTasks returns all tasks from the database.
func (a *App) LoadTasks() []Task {
	if a.db == nil {
		return []Task{}
	}
	tasks, err := LoadTasks(a.db)
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
		`SELECT id, title, prompt, spec, branch, pr_url, status, approved FROM tasks WHERE id=?`, id,
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

// DeleteTask removes a task by ID.
func (a *App) DeleteTask(id int) error {
	if a.db == nil {
		return fmt.Errorf("database not initialised")
	}
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
	migrateFromJSON(abs, db)

	a.emitTasks()
	a.log(fmt.Sprintf("[CONFIG] Working directory set to: %s", abs))
	return nil
}

// GetSettings returns all user-configurable settings as a map.
func (a *App) GetSettings() (map[string]string, error) {
	result := map[string]string{
		"workDir": a.workDir,
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

		task, found, err := findActionableDB(a.db)
		if err != nil {
			a.log(fmt.Sprintf("[ERROR] %v", err))
			continue
		}
		if !found {
			continue
		}

		var handlerErr error
		switch task.Status {
		case "prompt":
			handlerErr = handlePrompt(task, dir, a.db, a.log)
		case "code":
			handlerErr = handleCode(task, dir, a.db, a.log)
		}

		if handlerErr != nil {
			a.log(fmt.Sprintf("[ERROR] Task #%d: %v", task.ID, handlerErr))
		}

		a.emitTasks()
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (a *App) emitTasks() {
	if a.ctx == nil {
		return
	}
	var tasks []Task
	if a.db != nil {
		var err error
		tasks, err = LoadTasks(a.db)
		if err != nil {
			tasks = []Task{}
		}
	}
	if tasks == nil {
		tasks = []Task{}
	}
	runtime.EventsEmit(a.ctx, "tasks:updated", tasks)
}

func (a *App) log(msg string) {
	fmt.Println(msg)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", msg)
	}
}
