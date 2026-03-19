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
	MigrateFromJSON(a.workDir, db)

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
	_, err := InsertTask(a.db, Task{
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
// review → sets status to "done" and approved to true
// prompt/code/done → just sets approved to true
func (a *App) ApproveTask(id int) error {
	if a.db == nil {
		return fmt.Errorf("database not initialised")
	}

	row := a.db.QueryRow(
		`SELECT id, title, prompt, spec, branch, status, approved FROM tasks WHERE id=?`, id,
	)
	task, err := scanTask(row)
	if err == sql.ErrNoRows {
		return fmt.Errorf("task #%d not found", id)
	}
	if err != nil {
		return err
	}

	newStatus := task.Status
	switch task.Status {
	case "spec":
		newStatus = "code"
	case "review":
		newStatus = "done"
	}

	if err := UpdateTaskStatus(a.db, id, newStatus, true); err != nil {
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
	if err := DeleteTask(a.db, id); err != nil {
		return err
	}
	a.emitTasks()
	return nil
}

// GetWorkDir returns the current working directory.
func (a *App) GetWorkDir() string {
	return a.workDir
}

// SetWorkDir opens a directory picker and sets the working directory.
func (a *App) SetWorkDir() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Select project directory",
		DefaultDirectory:     a.workDir,
		CanCreateDirectories: true,
	})
	if err != nil || dir == "" {
		return a.workDir, nil
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return a.workDir, err
	}

	a.mu.Lock()
	a.workDir = abs
	a.mu.Unlock()

	if a.db != nil {
		a.db.Close()
	}
	db, err := OpenDB(abs)
	if err != nil {
		return abs, err
	}
	a.db = db
	MigrateFromJSON(abs, db)

	a.emitTasks()
	a.log(fmt.Sprintf("[CONFIG] Working directory set to: %s", abs))
	return abs, nil
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

		task, found, err := FindActionableDB(a.db)
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
		case "done":
			handlerErr = handleDone(task, dir, a.db, a.log)
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
