package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func testDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := openDB(dir)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	return db, func() { db.Close() }
}

func TestOpenDB_CreatesSchema(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	// Should be able to query the tasks table immediately
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&count); err != nil {
		t.Fatalf("tasks table not created: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows, got %d", count)
	}
}

func TestOpenDB_CreatesSettingsTable(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&count); err != nil {
		t.Fatalf("settings table not created: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows in settings, got %d", count)
	}
}

func TestGetSetSetting(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	// Missing key returns empty string, no error.
	val, err := getSetting(db, "missing")
	if err != nil {
		t.Fatalf("getSetting on missing key: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}

	// Set a value and read it back.
	if err := setSetting(db, "workDir", "/tmp/project"); err != nil {
		t.Fatalf("setSetting: %v", err)
	}
	val, err = getSetting(db, "workDir")
	if err != nil {
		t.Fatalf("getSetting: %v", err)
	}
	if val != "/tmp/project" {
		t.Errorf("expected %q, got %q", "/tmp/project", val)
	}

	// Upsert overwrites the previous value.
	if err := setSetting(db, "workDir", "/home/user"); err != nil {
		t.Fatalf("setSetting upsert: %v", err)
	}
	val, _ = getSetting(db, "workDir")
	if val != "/home/user" {
		t.Errorf("upsert: expected %q, got %q", "/home/user", val)
	}
}

func TestInsertAndLoadTasks(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, err := insertTask(db, Task{Title: "T1", Prompt: "P1", Status: "prompt", Approved: false})
	if err != nil {
		t.Fatalf("insertTask: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero inserted id")
	}

	tasks, err := loadTasks(db)
	if err != nil {
		t.Fatalf("loadTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "T1" || tasks[0].Status != "prompt" || tasks[0].Approved {
		t.Errorf("unexpected task fields: %+v", tasks[0])
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := insertTask(db, Task{Title: "T", Status: "prompt", Approved: false})
	if err := updateTaskStatus(db, int(id), "code", true); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}

	tasks, _ := loadTasks(db)
	if tasks[0].Status != "code" || !tasks[0].Approved {
		t.Errorf("unexpected fields after update: %+v", tasks[0])
	}
}

func TestUpdateTaskSpec(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := insertTask(db, Task{Title: "T", Status: "prompt"})
	if err := updateTaskSpec(db, int(id), "my spec"); err != nil {
		t.Fatalf("updateTaskSpec: %v", err)
	}

	tasks, _ := loadTasks(db)
	if tasks[0].Spec != "my spec" {
		t.Errorf("spec not updated: %q", tasks[0].Spec)
	}
}

func TestUpdateTaskBranch(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := insertTask(db, Task{Title: "T", Status: "code"})
	if err := updateTaskBranch(db, int(id), "feature/task-1-t"); err != nil {
		t.Fatalf("updateTaskBranch: %v", err)
	}

	tasks, _ := loadTasks(db)
	if tasks[0].Branch != "feature/task-1-t" {
		t.Errorf("branch not updated: %q", tasks[0].Branch)
	}
}

func TestDeleteTask(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := insertTask(db, Task{Title: "T", Status: "prompt"})
	if err := deleteTask(db, int(id)); err != nil {
		t.Fatalf("deleteTask: %v", err)
	}

	tasks, _ := loadTasks(db)
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(tasks))
	}
}

func TestFindActionableDB(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	// Not approved — should not be actionable
	insertTask(db, Task{Title: "A", Status: "prompt", Approved: false})

	task, found, err := findActionableDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("expected no actionable task, got %+v", task)
	}

	// Approve it
	tasks, _ := loadTasks(db)
	updateTaskStatus(db, tasks[0].ID, "prompt", true)

	task, found, err = findActionableDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected to find actionable task")
	}
	if task.Title != "A" {
		t.Errorf("wrong task returned: %+v", task)
	}
}

func TestFindActionableDB_SkipsNonActionableStatuses(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	// spec/review/complete are not actionable even if approved=1
	for _, status := range []string{"spec", "review", "complete", "failed"} {
		insertTask(db, Task{Title: status, Status: status, Approved: true})
	}

	_, found, err := findActionableDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected no actionable tasks for non-actionable statuses")
	}
}

func TestMigrateFromJSON(t *testing.T) {
	dir := t.TempDir()

	tasks := []Task{
		{ID: 1, Title: "Migrate me", Prompt: "P", Status: "prompt", Approved: false},
		{ID: 2, Title: "And me", Prompt: "Q", Status: "spec", Approved: false},
	}
	data, _ := json.Marshal(tasks)
	jsonPath := filepath.Join(dir, "KANBAN.json")
	os.WriteFile(jsonPath, data, 0644)

	db, err := openDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrateFromJSON(dir, db)

	// JSON should be renamed to .bak
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Error("KANBAN.json should have been renamed to .bak")
	}
	if _, err := os.Stat(jsonPath + ".bak"); err != nil {
		t.Error("KANBAN.json.bak should exist")
	}

	// Tasks should be in DB
	loaded, _ := loadTasks(db)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 migrated tasks, got %d", len(loaded))
	}
	if loaded[0].Title != "Migrate me" || loaded[1].Title != "And me" {
		t.Errorf("unexpected tasks: %+v", loaded)
	}
}

func TestMigrateFromJSON_NoDoubleImport(t *testing.T) {
	dir := t.TempDir()

	tasks := []Task{{ID: 1, Title: "One", Status: "prompt"}}
	data, _ := json.Marshal(tasks)
	jsonPath := filepath.Join(dir, "KANBAN.json")
	os.WriteFile(jsonPath, data, 0644)

	db, _ := openDB(dir)
	defer db.Close()

	// Pre-populate DB
	insertTask(db, Task{Title: "Existing", Status: "prompt"})

	migrateFromJSON(dir, db)

	loaded, _ := loadTasks(db)
	// Only the pre-existing task should be there
	if len(loaded) != 1 || loaded[0].Title != "Existing" {
		t.Errorf("double import occurred: %+v", loaded)
	}
	// JSON should still be renamed to .bak
	if _, err := os.Stat(jsonPath + ".bak"); err != nil {
		t.Error("KANBAN.json.bak should exist even when DB not empty")
	}
}

func TestConcurrentWrites(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := insertTask(db, Task{
				Title:  "Concurrent task",
				Status: "prompt",
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent insert error: %v", err)
	}

	tasks, err := loadTasks(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 20 {
		t.Errorf("expected 20 tasks from concurrent inserts, got %d", len(tasks))
	}
}

func TestGetSetting_MissingKey(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	val, err := getSetting(db, "nonexistent")
	if err != nil {
		t.Fatalf("getSetting: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}
}

func TestSetAndGetSetting(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	if err := setSetting(db, "workDir", "/tmp/project"); err != nil {
		t.Fatalf("setSetting: %v", err)
	}

	val, err := getSetting(db, "workDir")
	if err != nil {
		t.Fatalf("getSetting: %v", err)
	}
	if val != "/tmp/project" {
		t.Errorf("expected /tmp/project, got %q", val)
	}
}

func TestSetSetting_Upsert(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	setSetting(db, "workDir", "/tmp/first")
	if err := setSetting(db, "workDir", "/tmp/second"); err != nil {
		t.Fatalf("setSetting upsert: %v", err)
	}

	val, _ := getSetting(db, "workDir")
	if val != "/tmp/second" {
		t.Errorf("expected upserted value /tmp/second, got %q", val)
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should be 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should be 0")
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"Add health check", "add-health-check"},
		{"  leading-trailing  ", "leading-trailing"},
		{"MiXeD CaSe", "mixed-case"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
