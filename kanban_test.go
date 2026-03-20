package main

import (
	"database/sql"
	"sync"
	"testing"

	"github.com/rengas/twist/pkg"
)

func testDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := pkg.OpenDB(dir)
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
	val, err := pkg.GetSetting(db, "missing")
	if err != nil {
		t.Fatalf("getSetting on missing key: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}

	// Set a value and read it back.
	if err := pkg.SetSetting(db, "workDir", "/tmp/project"); err != nil {
		t.Fatalf("setSetting: %v", err)
	}
	val, err = pkg.GetSetting(db, "workDir")
	if err != nil {
		t.Fatalf("getSetting: %v", err)
	}
	if val != "/tmp/project" {
		t.Errorf("expected %q, got %q", "/tmp/project", val)
	}

	// Upsert overwrites the previous value.
	if err := pkg.SetSetting(db, "workDir", "/home/user"); err != nil {
		t.Fatalf("setSetting upsert: %v", err)
	}
	val, _ = pkg.GetSetting(db, "workDir")
	if val != "/home/user" {
		t.Errorf("upsert: expected %q, got %q", "/home/user", val)
	}
}

func TestInsertAndLoadTasks(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, err := pkg.InsertTask(db, pkg.Task{Title: "T1", Prompt: "P1", Status: "prompt", Approved: false})
	if err != nil {
		t.Fatalf("insertTask: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero inserted id")
	}

	tasks, err := pkg.LoadTasks(db)
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

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "prompt", Approved: false})
	if err := pkg.UpdateTaskStatus(db, int(id), "code", true); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}

	tasks, _ := pkg.LoadTasks(db)
	if tasks[0].Status != "code" || !tasks[0].Approved {
		t.Errorf("unexpected fields after update: %+v", tasks[0])
	}
}

func TestUpdateTaskSpec(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "prompt"})
	if err := pkg.UpdateTaskSpec(db, int(id), "my spec"); err != nil {
		t.Fatalf("updateTaskSpec: %v", err)
	}

	tasks, _ := pkg.LoadTasks(db)
	if tasks[0].Spec != "my spec" {
		t.Errorf("spec not updated: %q", tasks[0].Spec)
	}
}

func TestUpdateTaskBranch(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "code"})
	if err := pkg.UpdateTaskBranch(db, int(id), "feature/task-1-t"); err != nil {
		t.Fatalf("updateTaskBranch: %v", err)
	}

	tasks, _ := pkg.LoadTasks(db)
	if tasks[0].Branch != "feature/task-1-t" {
		t.Errorf("branch not updated: %q", tasks[0].Branch)
	}
}

func TestDeleteTask(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "prompt"})
	if err := pkg.DeleteTask(db, int(id)); err != nil {
		t.Fatalf("deleteTask: %v", err)
	}

	tasks, _ := pkg.LoadTasks(db)
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(tasks))
	}
}

func TestFindActionableDB(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	// Not approved — should not be actionable
	pkg.InsertTask(db, pkg.Task{Title: "A", Status: "prompt", Approved: false})

	task, found, err := pkg.FindActionableDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("expected no actionable task, got %+v", task)
	}

	// Approve it
	tasks, _ := pkg.LoadTasks(db)
	pkg.UpdateTaskStatus(db, tasks[0].ID, "prompt", true)

	task, found, err = pkg.FindActionableDB(db)
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

	// spec/review/done/failed are not actionable even if approved=1
	for _, status := range []string{"spec", "review", "done", "failed"} {
		pkg.InsertTask(db, pkg.Task{Title: status, Status: status, Approved: true})
	}

	_, found, err := pkg.FindActionableDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected no actionable tasks for non-actionable statuses")
	}
}

func TestUpdateTaskPRURL(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "code"})
	if err := pkg.UpdateTaskPRURL(db, int(id), "https://github.com/org/repo/pull/42"); err != nil {
		t.Fatalf("updateTaskPRURL: %v", err)
	}

	tasks, _ := pkg.LoadTasks(db)
	if tasks[0].PRURL != "https://github.com/org/repo/pull/42" {
		t.Errorf("pr_url not updated: %q", tasks[0].PRURL)
	}
}

func TestDoneIsTerminal(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	// A task in "done" status with approved=true should NOT be actionable
	pkg.InsertTask(db, pkg.Task{Title: "Done task", Status: "done", Approved: true})

	_, found, err := pkg.FindActionableDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("tasks in 'done' status should not be actionable — done is terminal")
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
			_, err := pkg.InsertTask(db, pkg.Task{
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

	tasks, err := pkg.LoadTasks(db)
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

	val, err := pkg.GetSetting(db, "nonexistent")
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

	if err := pkg.SetSetting(db, "workDir", "/tmp/project"); err != nil {
		t.Fatalf("setSetting: %v", err)
	}

	val, err := pkg.GetSetting(db, "workDir")
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

	pkg.SetSetting(db, "workDir", "/tmp/first")
	if err := pkg.SetSetting(db, "workDir", "/tmp/second"); err != nil {
		t.Fatalf("setSetting upsert: %v", err)
	}

	val, _ := pkg.GetSetting(db, "workDir")
	if val != "/tmp/second" {
		t.Errorf("expected upserted value /tmp/second, got %q", val)
	}
}

func TestBoolToInt(t *testing.T) {
	if pkg.BoolToInt(true) != 1 {
		t.Error("boolToInt(true) should be 1")
	}
	if pkg.BoolToInt(false) != 0 {
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
		if got := pkg.Slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── Phase 1: Session Persistence Tests ────────────────────────────────────────

func TestSessionID_Persistence(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "prompt"})
	if err := pkg.UpdateTaskSessionID(db, int(id), "test-session-123"); err != nil {
		t.Fatalf("updateTaskSessionID: %v", err)
	}

	task, err := pkg.GetTaskByID(db, int(id))
	if err != nil {
		t.Fatalf("getTaskByID: %v", err)
	}
	if task.SessionID != "test-session-123" {
		t.Errorf("session_id not updated: %q", task.SessionID)
	}
}

func TestSessionID_InsertedWithTask(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "prompt", SessionID: "pre-set-id"})
	task, _ := pkg.GetTaskByID(db, int(id))
	if task.SessionID != "pre-set-id" {
		t.Errorf("expected session_id to be pre-set-id, got %q", task.SessionID)
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid1 := pkg.GenerateUUID()
	uuid2 := pkg.GenerateUUID()

	if len(uuid1) == 0 {
		t.Fatal("UUID should not be empty")
	}
	if uuid1 == uuid2 {
		t.Error("two UUIDs should not be identical")
	}
	// UUID v4 format: 8-4-4-4-12 chars
	if len(uuid1) != 36 {
		t.Errorf("UUID should be 36 chars, got %d: %q", len(uuid1), uuid1)
	}
}

func TestSchemaMigration_SessionIDAndWorktreePath(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	// Verify session_id and worktree_path columns exist by querying them directly.
	var sid, wtp string
	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "prompt"})
	err := db.QueryRow(`SELECT session_id, worktree_path FROM tasks WHERE id=?`, id).Scan(&sid, &wtp)
	if err != nil {
		t.Fatalf("new columns should exist: %v", err)
	}
	if sid != "" || wtp != "" {
		t.Errorf("defaults should be empty strings, got sid=%q wtp=%q", sid, wtp)
	}
}

// ── Phase 2: Worktree Path Tests ──────────────────────────────────────────────

func TestWorktreePath_Persistence(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "T", Status: "code"})
	if err := pkg.UpdateTaskWorktreePath(db, int(id), "/tmp/wt/task-1"); err != nil {
		t.Fatalf("updateTaskWorktreePath: %v", err)
	}

	task, _ := pkg.GetTaskByID(db, int(id))
	if task.WorktreePath != "/tmp/wt/task-1" {
		t.Errorf("worktree_path not updated: %q", task.WorktreePath)
	}
}

// ── Phase 3: Parallel Execution Tests ─────────────────────────────────────────

func TestFindActionableTasksDB_ReturnsMultiple(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	// Insert 3 actionable tasks.
	pkg.InsertTask(db, pkg.Task{Title: "A", Status: "prompt", Approved: true})
	pkg.InsertTask(db, pkg.Task{Title: "B", Status: "code", Approved: true})
	pkg.InsertTask(db, pkg.Task{Title: "C", Status: "prompt", Approved: true})

	// Insert non-actionable tasks.
	pkg.InsertTask(db, pkg.Task{Title: "D", Status: "spec", Approved: true})
	pkg.InsertTask(db, pkg.Task{Title: "E", Status: "prompt", Approved: false})

	tasks, err := pkg.FindActionableTasksDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 actionable tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "A" || tasks[1].Title != "B" || tasks[2].Title != "C" {
		t.Errorf("unexpected task order: %+v", tasks)
	}
}

func TestFindActionableTasksDB_Empty(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	tasks, err := pkg.FindActionableTasksDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

// ── Phase 4: Design Document Tests ───────────────────────────────────────────

func TestDesignVersions_Schema(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM design_versions`).Scan(&count); err != nil {
		t.Fatalf("design_versions table not created: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows, got %d", count)
	}
}

func TestDesignVersion_AppendAndGet(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	dir := t.TempDir()
	var mu sync.Mutex

	pkg.AppendDesignVersion(db, &mu, dir, 1, "## Task #1\nFirst spec", "Task 1 spec")

	version, content, err := pkg.GetLatestDesignVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
	if content != "## Task #1\nFirst spec" {
		t.Errorf("unexpected content: %q", content)
	}

	// Append another version.
	pkg.AppendDesignVersion(db, &mu, dir, 2, "## Task #2\nSecond spec", "Task 2 spec")

	version, content, err = pkg.GetLatestDesignVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Errorf("expected version 2, got %d", version)
	}
	if !containsSubstring(content, "Task #1") || !containsSubstring(content, "Task #2") {
		t.Errorf("content should contain both tasks: %q", content)
	}
}

func TestDesignHistory(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	dir := t.TempDir()
	var mu sync.Mutex

	pkg.AppendDesignVersion(db, &mu, dir, 1, "Section 1", "First")
	pkg.AppendDesignVersion(db, &mu, dir, 2, "Section 2", "Second")

	history, err := pkg.GetDesignHistory(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(history))
	}
	// History is DESC order.
	if history[0].Version != 2 || history[1].Version != 1 {
		t.Errorf("unexpected version order: %+v", history)
	}
	if history[0].Summary != "Second" || history[1].Summary != "First" {
		t.Errorf("unexpected summaries: %+v", history)
	}
}

func TestDesignVersion_ConcurrentWrites(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	dir := t.TempDir()
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pkg.AppendDesignVersion(db, &mu, dir, n, "Section", "summary")
		}(i)
	}
	wg.Wait()

	history, err := pkg.GetDesignHistory(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 10 {
		t.Fatalf("expected 10 versions from concurrent writes, got %d", len(history))
	}

	// Verify versions are monotonically increasing (no gaps, no dupes).
	versions := make(map[int]bool)
	for _, v := range history {
		versions[v.Version] = true
	}
	for i := 1; i <= 10; i++ {
		if !versions[i] {
			t.Errorf("missing version %d", i)
		}
	}
}

func TestBuildTaskContext(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	pkg.InsertTask(db, pkg.Task{Title: "Task A", Spec: "Spec for A", Status: "spec"})
	pkg.InsertTask(db, pkg.Task{Title: "Task B", Spec: "Spec for B", Status: "code"})
	id3, _ := pkg.InsertTask(db, pkg.Task{Title: "Task C", Spec: "Spec for C", Status: "prompt"})

	// Exclude task C — should only see A and B.
	ctx, err := pkg.BuildTaskContext(db, int(id3))
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(ctx, "Task A") || !containsSubstring(ctx, "Task B") {
		t.Errorf("context should include A and B: %q", ctx)
	}
	if containsSubstring(ctx, "Task C") {
		t.Error("context should exclude the current task")
	}
}

func TestBuildTaskContext_ExcludesEmptySpecs(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	pkg.InsertTask(db, pkg.Task{Title: "No Spec", Spec: "", Status: "prompt"})
	pkg.InsertTask(db, pkg.Task{Title: "Has Spec", Spec: "details", Status: "spec"})

	ctx, _ := pkg.BuildTaskContext(db, 0)
	if containsSubstring(ctx, "No Spec") {
		t.Error("should not include tasks with empty specs")
	}
	if !containsSubstring(ctx, "Has Spec") {
		t.Error("should include tasks with specs")
	}
}

func TestTruncate(t *testing.T) {
	if got := pkg.Truncate("hello", 10); got != "hello" {
		t.Errorf("short string should not be truncated: %q", got)
	}
	if got := pkg.Truncate("hello world", 5); got != "hello..." {
		t.Errorf("long string should be truncated: %q", got)
	}
}

func TestGetTaskByID(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	id, _ := pkg.InsertTask(db, pkg.Task{Title: "Lookup", Status: "prompt", Prompt: "test prompt"})
	task, err := pkg.GetTaskByID(db, int(id))
	if err != nil {
		t.Fatalf("getTaskByID: %v", err)
	}
	if task.Title != "Lookup" || task.Prompt != "test prompt" {
		t.Errorf("unexpected task: %+v", task)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
