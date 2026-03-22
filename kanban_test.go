package main

import (
	"os"
	"sync"
	"testing"

	"github.com/rengas/twist/pkg"
)

func testDBURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TWIST_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost:5432/twist_test?sslmode=disable"
	}
	return url
}

func testRepo(t *testing.T) (*pkg.PostgresRepository, func()) {
	t.Helper()
	url := testDBURL(t)
	repo, err := pkg.NewPostgresRepository(url)
	if err != nil {
		t.Skipf("skipping: cannot connect to postgres at %s: %v", url, err)
	}
	// Run migrations to ensure schema exists.
	if _, err := pkg.RunMigrations(url, nil); err != nil {
		repo.Close()
		t.Skipf("skipping: migration failed: %v", err)
	}
	// Truncate before test to ensure clean state.
	repo.TruncateAll()
	cleanup := func() {
		repo.TruncateAll()
		repo.Close()
	}
	return repo, cleanup
}

// ── Schema Tests ──────────────────────────────────────────────────────────────

func TestPostgres_Connect(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	if err := repo.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestPostgres_CreateSchema(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	// Schema should already be created by NewPostgresRepository.
	// Verify by loading tasks (empty table).
	tasks, err := repo.LoadTasks()
	if err != nil {
		t.Fatalf("tasks table not usable: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 rows, got %d", len(tasks))
	}
}

// ── Settings Tests ────────────────────────────────────────────────────────────

func TestPostgres_GetSetting_MissingKey(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	val, err := repo.GetSetting("nonexistent")
	if err != nil {
		t.Fatalf("getSetting: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}
}

func TestPostgres_SetAndGetSetting(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	if err := repo.SetSetting("workDir", "/tmp/project"); err != nil {
		t.Fatalf("setSetting: %v", err)
	}

	val, err := repo.GetSetting("workDir")
	if err != nil {
		t.Fatalf("getSetting: %v", err)
	}
	if val != "/tmp/project" {
		t.Errorf("expected /tmp/project, got %q", val)
	}
}

func TestPostgres_SetSetting_Upsert(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	repo.SetSetting("workDir", "/tmp/first")
	if err := repo.SetSetting("workDir", "/tmp/second"); err != nil {
		t.Fatalf("setSetting upsert: %v", err)
	}

	val, _ := repo.GetSetting("workDir")
	if val != "/tmp/second" {
		t.Errorf("expected upserted value /tmp/second, got %q", val)
	}
}

// ── Task CRUD Tests ───────────────────────────────────────────────────────────

func TestPostgres_InsertAndLoadTasks(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, err := repo.InsertTask(pkg.Task{Title: "T1", Prompt: "P1", Status: "prompt", Approved: false})
	if err != nil {
		t.Fatalf("insertTask: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero inserted id")
	}

	tasks, err := repo.LoadTasks()
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

func TestPostgres_UpdateTaskStatus(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "prompt", Approved: false})
	if err := repo.UpdateTaskStatus(int(id), "code", true); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}

	tasks, _ := repo.LoadTasks()
	if tasks[0].Status != "code" || !tasks[0].Approved {
		t.Errorf("unexpected fields after update: %+v", tasks[0])
	}
}

func TestPostgres_UpdateTaskSpec(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "prompt"})
	if err := repo.UpdateTaskSpec(int(id), "my spec"); err != nil {
		t.Fatalf("updateTaskSpec: %v", err)
	}

	tasks, _ := repo.LoadTasks()
	if tasks[0].Spec != "my spec" {
		t.Errorf("spec not updated: %q", tasks[0].Spec)
	}
}

func TestPostgres_UpdateTaskBranch(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "code"})
	if err := repo.UpdateTaskBranch(int(id), "feature/task-1-t"); err != nil {
		t.Fatalf("updateTaskBranch: %v", err)
	}

	tasks, _ := repo.LoadTasks()
	if tasks[0].Branch != "feature/task-1-t" {
		t.Errorf("branch not updated: %q", tasks[0].Branch)
	}
}

func TestPostgres_UpdateTaskPRURL(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "code"})
	if err := repo.UpdateTaskPRURL(int(id), "https://github.com/org/repo/pull/42"); err != nil {
		t.Fatalf("updateTaskPRURL: %v", err)
	}

	tasks, _ := repo.LoadTasks()
	if tasks[0].PRURL != "https://github.com/org/repo/pull/42" {
		t.Errorf("pr_url not updated: %q", tasks[0].PRURL)
	}
}

func TestPostgres_DeleteTask(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "prompt"})
	if err := repo.DeleteTask(int(id)); err != nil {
		t.Fatalf("deleteTask: %v", err)
	}

	tasks, _ := repo.LoadTasks()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(tasks))
	}
}

func TestPostgres_GetTaskByID(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "Lookup", Status: "prompt", Prompt: "test prompt"})
	task, err := repo.GetTaskByID(int(id))
	if err != nil {
		t.Fatalf("getTaskByID: %v", err)
	}
	if task.Title != "Lookup" || task.Prompt != "test prompt" {
		t.Errorf("unexpected task: %+v", task)
	}
}

// ── Session & Worktree Tests ──────────────────────────────────────────────────

func TestPostgres_SessionID_Persistence(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "prompt"})
	if err := repo.UpdateTaskSessionID(int(id), "test-session-123"); err != nil {
		t.Fatalf("updateTaskSessionID: %v", err)
	}

	task, err := repo.GetTaskByID(int(id))
	if err != nil {
		t.Fatalf("getTaskByID: %v", err)
	}
	if task.SessionID != "test-session-123" {
		t.Errorf("session_id not updated: %q", task.SessionID)
	}
}

func TestPostgres_WorktreePath_Persistence(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "code"})
	if err := repo.UpdateTaskWorktreePath(int(id), "/tmp/wt/task-1"); err != nil {
		t.Fatalf("updateTaskWorktreePath: %v", err)
	}

	task, _ := repo.GetTaskByID(int(id))
	if task.WorktreePath != "/tmp/wt/task-1" {
		t.Errorf("worktree_path not updated: %q", task.WorktreePath)
	}
}

// ── Actionable Task Tests ─────────────────────────────────────────────────────

func TestPostgres_FindActionableTask(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	// Not approved — should not be actionable.
	repo.InsertTask(pkg.Task{Title: "A", Status: "prompt", Approved: false})

	task, found, err := repo.FindActionableTask()
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("expected no actionable task, got %+v", task)
	}

	// Approve it.
	tasks, _ := repo.LoadTasks()
	repo.UpdateTaskStatus(tasks[0].ID, "prompt", true)

	task, found, err = repo.FindActionableTask()
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

func TestPostgres_FindActionableTask_SkipsNonActionable(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	for _, status := range []string{"spec", "review", "done", "failed"} {
		repo.InsertTask(pkg.Task{Title: status, Status: status, Approved: true})
	}

	_, found, err := repo.FindActionableTask()
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected no actionable tasks for non-actionable statuses")
	}
}

func TestPostgres_FindActionableTasks_ReturnsMultiple(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	repo.InsertTask(pkg.Task{Title: "A", Status: "prompt", Approved: true})
	repo.InsertTask(pkg.Task{Title: "B", Status: "code", Approved: true})
	repo.InsertTask(pkg.Task{Title: "C", Status: "prompt", Approved: true})
	repo.InsertTask(pkg.Task{Title: "D", Status: "spec", Approved: true})
	repo.InsertTask(pkg.Task{Title: "E", Status: "prompt", Approved: false})

	tasks, err := repo.FindActionableTasks()
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

func TestPostgres_DoneIsTerminal(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	repo.InsertTask(pkg.Task{Title: "Done task", Status: "done", Approved: true})

	_, found, err := repo.FindActionableTask()
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("tasks in 'done' status should not be actionable")
	}
}

// ── Design Version Tests ──────────────────────────────────────────────────────

func TestPostgres_DesignVersion_AppendAndGet(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	dir := t.TempDir()
	var mu sync.Mutex

	pkg.AppendDesignVersion(repo, &mu, dir, 1, "## Task #1\nFirst spec", "Task 1 spec")

	version, content, err := repo.GetLatestDesignVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
	if content != "## Task #1\nFirst spec" {
		t.Errorf("unexpected content: %q", content)
	}

	pkg.AppendDesignVersion(repo, &mu, dir, 2, "## Task #2\nSecond spec", "Task 2 spec")

	version, content, err = repo.GetLatestDesignVersion()
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

func TestPostgres_DesignHistory(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	dir := t.TempDir()
	var mu sync.Mutex

	pkg.AppendDesignVersion(repo, &mu, dir, 1, "Section 1", "First")
	pkg.AppendDesignVersion(repo, &mu, dir, 2, "Section 2", "Second")

	history, err := repo.GetDesignHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(history))
	}
	if history[0].Version != 2 || history[1].Version != 1 {
		t.Errorf("unexpected version order: %+v", history)
	}
}

func TestPostgres_DesignVersion_ConcurrentWrites(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	dir := t.TempDir()
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pkg.AppendDesignVersion(repo, &mu, dir, n, "Section", "summary")
		}(i)
	}
	wg.Wait()

	history, err := repo.GetDesignHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 10 {
		t.Fatalf("expected 10 versions from concurrent writes, got %d", len(history))
	}

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

// ── Cross-task Context Tests ──────────────────────────────────────────────────

func TestPostgres_GetTaskSpecs(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	repo.InsertTask(pkg.Task{Title: "Task A", Spec: "Spec for A", Status: "spec"})
	repo.InsertTask(pkg.Task{Title: "Task B", Spec: "Spec for B", Status: "code"})
	id3, _ := repo.InsertTask(pkg.Task{Title: "Task C", Spec: "Spec for C", Status: "prompt"})

	specs, err := repo.GetTaskSpecs(int(id3), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
}

func TestPostgres_BuildTaskContext(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	repo.InsertTask(pkg.Task{Title: "Task A", Spec: "Spec for A", Status: "spec"})
	repo.InsertTask(pkg.Task{Title: "Task B", Spec: "Spec for B", Status: "code"})
	id3, _ := repo.InsertTask(pkg.Task{Title: "Task C", Spec: "Spec for C", Status: "prompt"})

	ctx := pkg.BuildTaskContext(repo, int(id3))
	if !containsSubstring(ctx, "Task A") || !containsSubstring(ctx, "Task B") {
		t.Errorf("context should include A and B: %q", ctx)
	}
	if containsSubstring(ctx, "Task C") {
		t.Error("context should exclude the current task")
	}
}

func TestPostgres_BuildTaskContext_ExcludesEmptySpecs(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	repo.InsertTask(pkg.Task{Title: "No Spec", Spec: "", Status: "prompt"})
	repo.InsertTask(pkg.Task{Title: "Has Spec", Spec: "details", Status: "spec"})

	ctx := pkg.BuildTaskContext(repo, 0)
	if containsSubstring(ctx, "No Spec") {
		t.Error("should not include tasks with empty specs")
	}
	if !containsSubstring(ctx, "Has Spec") {
		t.Error("should include tasks with specs")
	}
}

// ── Concurrent Insert Tests ───────────────────────────────────────────────────

func TestPostgres_ConcurrentInserts(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := repo.InsertTask(pkg.Task{
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

	tasks, err := repo.LoadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 20 {
		t.Errorf("expected 20 tasks from concurrent inserts, got %d", len(tasks))
	}
}

// ── Utility Tests (no DB needed) ──────────────────────────────────────────────

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

func TestGenerateUUID(t *testing.T) {
	uuid1 := pkg.GenerateUUID()
	uuid2 := pkg.GenerateUUID()

	if len(uuid1) == 0 {
		t.Fatal("UUID should not be empty")
	}
	if uuid1 == uuid2 {
		t.Error("two UUIDs should not be identical")
	}
	if len(uuid1) != 36 {
		t.Errorf("UUID should be 36 chars, got %d: %q", len(uuid1), uuid1)
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

// ── Chat Message Tests ────────────────────────────────────────────────────────

func TestPostgres_InsertAndGetChatMessages(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{Title: "Chat Task", Status: "done", Prompt: "test"})

	msg1, err := repo.InsertChatMessage(int(taskID), "user", "Hello Claude")
	if err != nil {
		t.Fatalf("insertChatMessage: %v", err)
	}
	if msg1.ID == 0 {
		t.Fatal("expected non-zero message ID")
	}
	if msg1.Role != "user" || msg1.Content != "Hello Claude" {
		t.Errorf("unexpected message: %+v", msg1)
	}

	msg2, err := repo.InsertChatMessage(int(taskID), "assistant", "Hi there!")
	if err != nil {
		t.Fatalf("insertChatMessage: %v", err)
	}
	if msg2.Role != "assistant" {
		t.Errorf("expected assistant role, got %q", msg2.Role)
	}

	msgs, err := repo.GetChatMessages(int(taskID))
	if err != nil {
		t.Fatalf("getChatMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Errorf("unexpected message order: %+v", msgs)
	}
}

func TestPostgres_GetChatMessages_Empty(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{Title: "No Chat", Status: "prompt"})

	msgs, err := repo.GetChatMessages(int(taskID))
	if err != nil {
		t.Fatalf("getChatMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestPostgres_ChatMessages_CascadeDelete(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{Title: "Delete Me", Status: "prompt"})
	repo.InsertChatMessage(int(taskID), "user", "test message")
	repo.InsertChatMessage(int(taskID), "assistant", "response")

	// Delete the task — chat messages should cascade.
	if err := repo.DeleteTask(int(taskID)); err != nil {
		t.Fatalf("deleteTask: %v", err)
	}

	msgs, err := repo.GetChatMessages(int(taskID))
	if err != nil {
		t.Fatalf("getChatMessages after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after cascade delete, got %d", len(msgs))
	}
}

func TestPostgres_ChatMessages_MultipleTasksIsolated(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskA, _ := repo.InsertTask(pkg.Task{Title: "Task A", Status: "done"})
	taskB, _ := repo.InsertTask(pkg.Task{Title: "Task B", Status: "done"})

	repo.InsertChatMessage(int(taskA), "user", "msg for A")
	repo.InsertChatMessage(int(taskA), "assistant", "reply for A")
	repo.InsertChatMessage(int(taskB), "user", "msg for B")

	msgsA, _ := repo.GetChatMessages(int(taskA))
	msgsB, _ := repo.GetChatMessages(int(taskB))

	if len(msgsA) != 2 {
		t.Errorf("expected 2 messages for task A, got %d", len(msgsA))
	}
	if len(msgsB) != 1 {
		t.Errorf("expected 1 message for task B, got %d", len(msgsB))
	}
}

// ── Chat Session ID Tests ─────────────────────────────────────────────────

func TestPostgres_ChatSessionID_Persistence(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "prompt"})

	// Initially empty.
	task, err := repo.GetTaskByID(int(id))
	if err != nil {
		t.Fatalf("getTaskByID: %v", err)
	}
	if task.ChatSessionID != "" {
		t.Errorf("expected empty chat_session_id, got %q", task.ChatSessionID)
	}

	// Update it.
	if err := repo.UpdateTaskChatSessionID(int(id), "chat-session-abc"); err != nil {
		t.Fatalf("updateTaskChatSessionID: %v", err)
	}

	task, err = repo.GetTaskByID(int(id))
	if err != nil {
		t.Fatalf("getTaskByID: %v", err)
	}
	if task.ChatSessionID != "chat-session-abc" {
		t.Errorf("chat_session_id not updated: %q", task.ChatSessionID)
	}
}

func TestPostgres_ChatSessionID_IndependentOfSessionID(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	id, _ := repo.InsertTask(pkg.Task{Title: "T", Status: "prompt"})

	// Set both session IDs independently.
	repo.UpdateTaskSessionID(int(id), "workflow-session-123")
	repo.UpdateTaskChatSessionID(int(id), "chat-session-456")

	task, _ := repo.GetTaskByID(int(id))
	if task.SessionID != "workflow-session-123" {
		t.Errorf("session_id wrong: %q", task.SessionID)
	}
	if task.ChatSessionID != "chat-session-456" {
		t.Errorf("chat_session_id wrong: %q", task.ChatSessionID)
	}
}

func TestPostgres_ChatSessionID_SurvivesLoadTasks(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	repo.InsertTask(pkg.Task{Title: "T", Status: "prompt"})
	tasks, _ := repo.LoadTasks()
	repo.UpdateTaskChatSessionID(tasks[0].ID, "chat-sess-load")

	tasks, err := repo.LoadTasks()
	if err != nil {
		t.Fatalf("loadTasks: %v", err)
	}
	if tasks[0].ChatSessionID != "chat-sess-load" {
		t.Errorf("chat_session_id not loaded: %q", tasks[0].ChatSessionID)
	}
}

func TestBuildChatContextMessage(t *testing.T) {
	msg := pkg.BuildChatContextMessage("Fix login bug", "The login form crashes on submit", "How should I test this?")

	if !containsSubstring(msg, "Fix login bug") {
		t.Error("context message should contain task title")
	}
	if !containsSubstring(msg, "The login form crashes on submit") {
		t.Error("context message should contain spec")
	}
	if !containsSubstring(msg, "How should I test this?") {
		t.Error("context message should contain user message")
	}
}

func TestBuildChatContextMessage_EmptySpec(t *testing.T) {
	msg := pkg.BuildChatContextMessage("New Task", "", "Hello")

	if !containsSubstring(msg, "New Task") {
		t.Error("context message should contain task title even with empty spec")
	}
	if !containsSubstring(msg, "Hello") {
		t.Error("context message should contain user message")
	}
}

// ── Task Event Tests ──────────────────────────────────────────────────────────

func TestPostgres_InsertAndGetTaskEvents(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{Title: "Event Task", Status: "prompt", Prompt: "test"})

	if err := repo.InsertTaskEvent(int(taskID), "prompt_submitted", "user", "Prompt submitted", "test prompt"); err != nil {
		t.Fatalf("insertTaskEvent: %v", err)
	}
	if err := repo.InsertTaskEvent(int(taskID), "spec_generated", "agent", "Spec generated by Claude", "spec content"); err != nil {
		t.Fatalf("insertTaskEvent: %v", err)
	}

	events, err := repo.GetTaskEvents(int(taskID))
	if err != nil {
		t.Fatalf("getTaskEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != "prompt_submitted" || events[0].Actor != "user" {
		t.Errorf("unexpected first event: %+v", events[0])
	}
	if events[1].EventType != "spec_generated" || events[1].Actor != "agent" {
		t.Errorf("unexpected second event: %+v", events[1])
	}
}

func TestPostgres_GetTaskEvents_Empty(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{Title: "No Events", Status: "prompt"})

	events, err := repo.GetTaskEvents(int(taskID))
	if err != nil {
		t.Fatalf("getTaskEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestPostgres_TaskEvents_CascadeDelete(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{Title: "Delete Me", Status: "prompt", Prompt: "test"})
	repo.InsertTaskEvent(int(taskID), "prompt_submitted", "user", "Prompt submitted", "test")

	if err := repo.DeleteTask(int(taskID)); err != nil {
		t.Fatalf("deleteTask: %v", err)
	}

	events, err := repo.GetTaskEvents(int(taskID))
	if err != nil {
		t.Fatalf("getTaskEvents after delete: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events after cascade delete, got %d", len(events))
	}
}

func TestPostgres_TaskEvents_MultipleTasksIsolated(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskA, _ := repo.InsertTask(pkg.Task{Title: "Task A", Status: "prompt"})
	taskB, _ := repo.InsertTask(pkg.Task{Title: "Task B", Status: "prompt"})

	repo.InsertTaskEvent(int(taskA), "prompt_submitted", "user", "Prompt A", "a")
	repo.InsertTaskEvent(int(taskA), "spec_generated", "agent", "Spec A", "a spec")
	repo.InsertTaskEvent(int(taskB), "prompt_submitted", "user", "Prompt B", "b")

	eventsA, _ := repo.GetTaskEvents(int(taskA))
	eventsB, _ := repo.GetTaskEvents(int(taskB))

	if len(eventsA) != 2 {
		t.Errorf("expected 2 events for task A, got %d", len(eventsA))
	}
	if len(eventsB) != 1 {
		t.Errorf("expected 1 event for task B, got %d", len(eventsB))
	}
}

// ── Chat Timeline Tests ──────────────────────────────────────────────────────

func TestPostgres_GetChatTimeline(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{Title: "Timeline Task", Status: "done", Prompt: "test"})

	// Insert events and messages.
	repo.InsertTaskEvent(int(taskID), "prompt_submitted", "user", "Prompt submitted", "test prompt")
	repo.InsertTaskEvent(int(taskID), "spec_generated", "agent", "Spec generated", "spec content")
	repo.InsertChatMessage(int(taskID), "user", "Hello Claude")
	repo.InsertChatMessage(int(taskID), "assistant", "Hi there!")

	timeline, err := repo.GetChatTimeline(int(taskID))
	if err != nil {
		t.Fatalf("getChatTimeline: %v", err)
	}
	if len(timeline) != 4 {
		t.Fatalf("expected 4 timeline entries, got %d", len(timeline))
	}

	// Verify types are correct.
	eventCount := 0
	messageCount := 0
	for _, entry := range timeline {
		switch entry.Type {
		case "event":
			eventCount++
			if entry.Event == nil {
				t.Error("event entry should have non-nil Event")
			}
		case "message":
			messageCount++
			if entry.Message == nil {
				t.Error("message entry should have non-nil Message")
			}
		default:
			t.Errorf("unexpected entry type: %s", entry.Type)
		}
	}
	if eventCount != 2 {
		t.Errorf("expected 2 event entries, got %d", eventCount)
	}
	if messageCount != 2 {
		t.Errorf("expected 2 message entries, got %d", messageCount)
	}
}

func TestPostgres_GetChatTimeline_Empty(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{Title: "Empty Timeline", Status: "prompt"})

	timeline, err := repo.GetChatTimeline(int(taskID))
	if err != nil {
		t.Fatalf("getChatTimeline: %v", err)
	}
	if len(timeline) != 0 {
		t.Errorf("expected 0 timeline entries, got %d", len(timeline))
	}
}

// ── Backfill Tests ───────────────────────────────────────────────────────────

func TestPostgres_BackfillTaskEvents_FullTask(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	// Create a task that looks like it went through the full pipeline.
	taskID, _ := repo.InsertTask(pkg.Task{
		Title:  "Backfill Task",
		Status: "done",
		Prompt: "implement feature X",
		Spec:   "detailed spec for X",
		PRURL:  "https://github.com/org/repo/pull/42",
	})

	if err := repo.BackfillTaskEvents(int(taskID)); err != nil {
		t.Fatalf("backfillTaskEvents: %v", err)
	}

	events, err := repo.GetTaskEvents(int(taskID))
	if err != nil {
		t.Fatalf("getTaskEvents: %v", err)
	}

	// Should have: prompt_submitted, spec_generated, spec_approved, code_completed, pr_created, review_approved
	if len(events) != 6 {
		t.Fatalf("expected 6 backfilled events, got %d", len(events))
	}

	expectedTypes := []string{"prompt_submitted", "spec_generated", "spec_approved", "code_completed", "pr_created", "review_approved"}
	for i, expected := range expectedTypes {
		if events[i].EventType != expected {
			t.Errorf("event[%d]: expected type %q, got %q", i, expected, events[i].EventType)
		}
	}

	// Verify content is populated.
	if events[0].Content != "implement feature X" {
		t.Errorf("prompt event should have prompt content, got %q", events[0].Content)
	}
	if events[1].Content != "detailed spec for X" {
		t.Errorf("spec event should have spec content, got %q", events[1].Content)
	}
}

func TestPostgres_BackfillTaskEvents_Idempotent(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{
		Title:  "Idempotent Task",
		Status: "done",
		Prompt: "test prompt",
		Spec:   "test spec",
		PRURL:  "https://github.com/org/repo/pull/1",
	})

	// First backfill.
	if err := repo.BackfillTaskEvents(int(taskID)); err != nil {
		t.Fatalf("first backfill: %v", err)
	}

	events1, _ := repo.GetTaskEvents(int(taskID))

	// Second backfill — should be a no-op.
	if err := repo.BackfillTaskEvents(int(taskID)); err != nil {
		t.Fatalf("second backfill: %v", err)
	}

	events2, _ := repo.GetTaskEvents(int(taskID))

	if len(events1) != len(events2) {
		t.Errorf("backfill should be idempotent: first=%d, second=%d", len(events1), len(events2))
	}
}

func TestPostgres_BackfillTaskEvents_PartialTask(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	// Task with only prompt (still in spec stage).
	taskID, _ := repo.InsertTask(pkg.Task{
		Title:  "Partial Task",
		Status: "spec",
		Prompt: "do something",
		Spec:   "spec for something",
	})

	if err := repo.BackfillTaskEvents(int(taskID)); err != nil {
		t.Fatalf("backfillTaskEvents: %v", err)
	}

	events, err := repo.GetTaskEvents(int(taskID))
	if err != nil {
		t.Fatalf("getTaskEvents: %v", err)
	}

	// Should have: prompt_submitted, spec_generated (no spec_approved since status is 'spec')
	if len(events) != 2 {
		t.Fatalf("expected 2 backfilled events for partial task, got %d", len(events))
	}
	if events[0].EventType != "prompt_submitted" {
		t.Errorf("expected prompt_submitted, got %q", events[0].EventType)
	}
	if events[1].EventType != "spec_generated" {
		t.Errorf("expected spec_generated, got %q", events[1].EventType)
	}
}

func TestPostgres_BackfillTaskEvents_NoPrompt(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	// Task with no prompt — should not backfill.
	taskID, _ := repo.InsertTask(pkg.Task{
		Title:  "No Prompt Task",
		Status: "prompt",
	})

	if err := repo.BackfillTaskEvents(int(taskID)); err != nil {
		t.Fatalf("backfillTaskEvents: %v", err)
	}

	events, _ := repo.GetTaskEvents(int(taskID))
	if len(events) != 0 {
		t.Errorf("expected 0 events for task with no prompt, got %d", len(events))
	}
}

func TestPostgres_BackfillTaskEvents_ExistingEventsNotOverwritten(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()

	taskID, _ := repo.InsertTask(pkg.Task{
		Title:  "Has Events",
		Status: "done",
		Prompt: "test",
		Spec:   "spec",
		PRURL:  "https://example.com/pr/1",
	})

	// Insert a manual event first.
	repo.InsertTaskEvent(int(taskID), "prompt_submitted", "user", "Prompt submitted", "test")

	// Backfill should be a no-op since events already exist.
	if err := repo.BackfillTaskEvents(int(taskID)); err != nil {
		t.Fatalf("backfillTaskEvents: %v", err)
	}

	events, _ := repo.GetTaskEvents(int(taskID))
	if len(events) != 1 {
		t.Errorf("expected 1 event (no backfill), got %d", len(events))
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
