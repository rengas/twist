package pkg

import (
	"database/sql"
	"fmt"
	"sort"

	_ "github.com/lib/pq"
)

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository opens a connection to PostgreSQL and pings it.
// Callers must run migrations before using the repo for queries.
func NewPostgresRepository(connURL string) (*PostgresRepository, error) {
	db, err := sql.Open("postgres", connURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	repo := &PostgresRepository{db: db}
	if err := repo.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}
	return repo, nil
}

func (r *PostgresRepository) Ping() error {
	return r.db.Ping()
}

func (r *PostgresRepository) Close() error {
	return r.db.Close()
}

// TruncateAll removes all data from all tables. Used by tests for clean state.
func (r *PostgresRepository) TruncateAll() {
	r.db.Exec(`TRUNCATE task_events, chat_messages, tasks, settings, design_versions, project_chat_messages, project_chats RESTART IDENTITY CASCADE`)
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

const pgTaskColumns = `id, title, prompt, spec, branch, pr_url, status, approved, session_id, chat_session_id, worktree_path`

func scanTaskRow(scanner interface {
	Scan(dest ...any) error
}) (Task, error) {
	var t Task
	err := scanner.Scan(
		&t.ID, &t.Title, &t.Prompt, &t.Spec, &t.Branch, &t.PRURL,
		&t.Status, &t.Approved, &t.SessionID, &t.ChatSessionID, &t.WorktreePath,
	)
	return t, err
}

func (r *PostgresRepository) InsertTask(t Task) (int64, error) {
	var id int64
	err := r.db.QueryRow(
		`INSERT INTO tasks (title, prompt, spec, branch, pr_url, status, approved, session_id, chat_session_id, worktree_path)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		t.Title, t.Prompt, t.Spec, t.Branch, t.PRURL, t.Status, t.Approved, t.SessionID, t.ChatSessionID, t.WorktreePath,
	).Scan(&id)
	return id, err
}

func (r *PostgresRepository) LoadTasks() ([]Task, error) {
	rows, err := r.db.Query(`SELECT ` + pgTaskColumns + ` FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (r *PostgresRepository) GetTaskByID(id int) (Task, error) {
	row := r.db.QueryRow(`SELECT `+pgTaskColumns+` FROM tasks WHERE id=$1`, id)
	return scanTaskRow(row)
}

func (r *PostgresRepository) UpdateTaskStatus(id int, status string, approved bool) error {
	_, err := r.db.Exec(`UPDATE tasks SET status=$1, approved=$2 WHERE id=$3`, status, approved, id)
	return err
}

func (r *PostgresRepository) UpdateTaskSpec(id int, spec string) error {
	_, err := r.db.Exec(`UPDATE tasks SET spec=$1 WHERE id=$2`, spec, id)
	return err
}

func (r *PostgresRepository) UpdateTaskBranch(id int, branch string) error {
	_, err := r.db.Exec(`UPDATE tasks SET branch=$1 WHERE id=$2`, branch, id)
	return err
}

func (r *PostgresRepository) UpdateTaskPRURL(id int, prURL string) error {
	_, err := r.db.Exec(`UPDATE tasks SET pr_url=$1 WHERE id=$2`, prURL, id)
	return err
}

func (r *PostgresRepository) UpdateTaskSessionID(id int, sessionID string) error {
	_, err := r.db.Exec(`UPDATE tasks SET session_id=$1 WHERE id=$2`, sessionID, id)
	return err
}

func (r *PostgresRepository) UpdateTaskChatSessionID(id int, chatSessionID string) error {
	_, err := r.db.Exec(`UPDATE tasks SET chat_session_id=$1 WHERE id=$2`, chatSessionID, id)
	return err
}

func (r *PostgresRepository) UpdateTaskWorktreePath(id int, path string) error {
	_, err := r.db.Exec(`UPDATE tasks SET worktree_path=$1 WHERE id=$2`, path, id)
	return err
}

func (r *PostgresRepository) UpdateTaskFields(id int, title, prompt, spec string) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET title = $1, prompt = $2, spec = $3 WHERE id = $4`,
		title, prompt, spec, id,
	)
	return err
}

func (r *PostgresRepository) DeleteTask(id int) error {
	_, err := r.db.Exec(`DELETE FROM tasks WHERE id=$1`, id)
	return err
}

func (r *PostgresRepository) FindActionableTask() (Task, bool, error) {
	row := r.db.QueryRow(
		`SELECT `+pgTaskColumns+` FROM tasks
		 WHERE approved=TRUE AND status IN ('prompt','code')
		 ORDER BY id LIMIT 1`,
	)
	t, err := scanTaskRow(row)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	return t, err == nil, err
}

func (r *PostgresRepository) FindActionableTasks() ([]Task, error) {
	rows, err := r.db.Query(
		`SELECT `+pgTaskColumns+` FROM tasks
		 WHERE approved=TRUE AND status IN ('prompt','code')
		 ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (r *PostgresRepository) ArchiveTask(id int) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET status='archived', approved=false WHERE id=$1`, id)
	return err
}

func (r *PostgresRepository) RestoreTask(id int) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET status='prompt', approved=false,
		 spec='', branch='', pr_url='', session_id='', chat_session_id='', worktree_path=''
		 WHERE id=$1 AND status='archived'`, id)
	return err
}

// ── Settings ──────────────────────────────────────────────────────────────────

func (r *PostgresRepository) GetSetting(key string) (string, error) {
	var value string
	err := r.db.QueryRow(`SELECT value FROM settings WHERE key=$1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (r *PostgresRepository) SetSetting(key, value string) error {
	_, err := r.db.Exec(
		`INSERT INTO settings (key, value) VALUES ($1,$2)
		 ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value`,
		key, value,
	)
	return err
}

// ── Design Versions ───────────────────────────────────────────────────────────

func (r *PostgresRepository) GetLatestDesignVersion() (int, string, error) {
	var version int
	var content string
	err := r.db.QueryRow(
		`SELECT version, content FROM design_versions ORDER BY version DESC LIMIT 1`,
	).Scan(&version, &content)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	return version, content, err
}

func (r *PostgresRepository) InsertDesignVersion(version int, content string, taskID int, summary string) error {
	_, err := r.db.Exec(
		`INSERT INTO design_versions (version, content, task_id, summary) VALUES ($1,$2,$3,$4)`,
		version, content, taskID, summary,
	)
	return err
}

func (r *PostgresRepository) GetDesignHistory() ([]DesignVersion, error) {
	rows, err := r.db.Query(
		`SELECT id, version, content, task_id, created_at, summary
		 FROM design_versions ORDER BY version DESC`,
	)
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

// ── Cross-task Context ────────────────────────────────────────────────────────

// ── Chat Messages ─────────────────────────────────────────────────────────────

func (r *PostgresRepository) InsertChatMessage(taskID int, role, content string) (ChatMessage, error) {
	var msg ChatMessage
	err := r.db.QueryRow(
		`INSERT INTO chat_messages (task_id, role, content) VALUES ($1, $2, $3)
		 RETURNING id, task_id, role, content, created_at`,
		taskID, role, content,
	).Scan(&msg.ID, &msg.TaskID, &msg.Role, &msg.Content, &msg.CreatedAt)
	return msg, err
}

func (r *PostgresRepository) GetChatMessages(taskID int) ([]ChatMessage, error) {
	rows, err := r.db.Query(
		`SELECT id, task_id, role, content, created_at FROM chat_messages
		 WHERE task_id = $1 ORDER BY created_at ASC`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.TaskID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// ── Cross-task Context ────────────────────────────────────────────────────────

// ── Task Events ───────────────────────────────────────────────────────────────

func (r *PostgresRepository) InsertTaskEvent(taskID int, eventType, actor, summary, content string) error {
	_, err := r.db.Exec(
		`INSERT INTO task_events (task_id, event_type, actor, summary, content) VALUES ($1, $2, $3, $4, $5)`,
		taskID, eventType, actor, summary, content,
	)
	return err
}

func (r *PostgresRepository) GetTaskEvents(taskID int) ([]TaskEvent, error) {
	rows, err := r.db.Query(
		`SELECT id, task_id, event_type, actor, summary, content, created_at
		 FROM task_events WHERE task_id = $1 ORDER BY created_at ASC`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []TaskEvent
	for rows.Next() {
		var e TaskEvent
		if err := rows.Scan(&e.ID, &e.TaskID, &e.EventType, &e.Actor, &e.Summary, &e.Content, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *PostgresRepository) GetChatTimeline(taskID int) ([]ChatTimelineEntry, error) {
	events, err := r.GetTaskEvents(taskID)
	if err != nil {
		return nil, err
	}
	messages, err := r.GetChatMessages(taskID)
	if err != nil {
		return nil, err
	}

	timeline := make([]ChatTimelineEntry, 0, len(events)+len(messages))
	for i := range events {
		timeline = append(timeline, ChatTimelineEntry{
			Type:      "event",
			Event:     &events[i],
			Timestamp: events[i].CreatedAt,
		})
	}
	for i := range messages {
		timeline = append(timeline, ChatTimelineEntry{
			Type:      "message",
			Message:   &messages[i],
			Timestamp: messages[i].CreatedAt,
		})
	}

	// Sort by timestamp ascending.
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Timestamp < timeline[j].Timestamp
	})
	return timeline, nil
}

func (r *PostgresRepository) BackfillTaskEvents(taskID int) error {
	// Check if any events already exist for this task.
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM task_events WHERE task_id = $1`, taskID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // Already has events, no backfill needed.
	}

	task, err := r.GetTaskByID(taskID)
	if err != nil {
		return err
	}

	// Only backfill if the task has a non-empty prompt.
	if task.Prompt == "" {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get task creation time as baseline.
	var createdAt string
	if err := tx.QueryRow(`SELECT COALESCE(
		(SELECT created_at FROM chat_messages WHERE task_id = $1 ORDER BY created_at ASC LIMIT 1),
		NOW()
	)`, taskID).Scan(&createdAt); err != nil {
		return err
	}

	insert := func(eventType, actor, summary, content, ts string) error {
		_, err := tx.Exec(
			`INSERT INTO task_events (task_id, event_type, actor, summary, content, created_at) VALUES ($1, $2, $3, $4, $5, $6::timestamp)`,
			taskID, eventType, actor, summary, content, ts,
		)
		return err
	}

	// 1. prompt_submitted
	if err := insert("prompt_submitted", "user", "Prompt submitted", task.Prompt, createdAt); err != nil {
		return err
	}

	// 2. spec_generated (if spec exists)
	if task.Spec != "" {
		if err := insert("spec_generated", "agent", "Spec generated by Claude", task.Spec, createdAt); err != nil {
			return err
		}
	}

	// 3. spec_approved (if status is code or later)
	statusOrder := map[string]int{"prompt": 0, "spec": 1, "code": 2, "review": 3, "done": 4, "failed": 2}
	if statusOrder[task.Status] >= 2 {
		if err := insert("spec_approved", "user", "Spec approved — moved to code", "", createdAt); err != nil {
			return err
		}
	}

	// 4. pr_created + code_completed (if PR URL exists)
	if task.PRURL != "" {
		if err := insert("code_completed", "agent", "Code implementation completed", "", createdAt); err != nil {
			return err
		}
		if err := insert("pr_created", "system", fmt.Sprintf("PR created: %s", task.PRURL), task.PRURL, createdAt); err != nil {
			return err
		}
	}

	// 5. review_approved (if status is done)
	if task.Status == "done" {
		if err := insert("review_approved", "user", "Review approved — moved to done", "", createdAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ── Project-level Chat ───────────────────────────────────────────────────────

func (r *PostgresRepository) InsertProjectChat() (int64, error) {
	var id int64
	err := r.db.QueryRow(
		`INSERT INTO project_chats (title) VALUES ('New Chat') RETURNING id`,
	).Scan(&id)
	return id, err
}

func (r *PostgresRepository) GetActiveProjectChat() (*ProjectChat, error) {
	var c ProjectChat
	err := r.db.QueryRow(
		`SELECT id, session_id, title, archived, created_at FROM project_chats
		 WHERE archived = FALSE ORDER BY id DESC LIMIT 1`,
	).Scan(&c.ID, &c.SessionID, &c.Title, &c.Archived, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresRepository) ArchiveProjectChat(id int) error {
	_, err := r.db.Exec(`UPDATE project_chats SET archived = TRUE WHERE id = $1`, id)
	return err
}

func (r *PostgresRepository) GetProjectChatByID(id int) (ProjectChat, error) {
	var c ProjectChat
	err := r.db.QueryRow(
		`SELECT id, session_id, title, archived, created_at FROM project_chats WHERE id = $1`, id,
	).Scan(&c.ID, &c.SessionID, &c.Title, &c.Archived, &c.CreatedAt)
	return c, err
}

func (r *PostgresRepository) UpdateProjectChatSessionID(id int, sessionID string) error {
	_, err := r.db.Exec(`UPDATE project_chats SET session_id = $1 WHERE id = $2`, sessionID, id)
	return err
}

func (r *PostgresRepository) UpdateProjectChatTitle(id int, title string) error {
	_, err := r.db.Exec(`UPDATE project_chats SET title = $1 WHERE id = $2`, title, id)
	return err
}

func (r *PostgresRepository) InsertProjectChatMessage(chatID int, role, content string) (ProjectChatMessage, error) {
	var msg ProjectChatMessage
	err := r.db.QueryRow(
		`INSERT INTO project_chat_messages (chat_id, role, content) VALUES ($1, $2, $3)
		 RETURNING id, chat_id, role, content, created_at`,
		chatID, role, content,
	).Scan(&msg.ID, &msg.ChatID, &msg.Role, &msg.Content, &msg.CreatedAt)
	return msg, err
}

func (r *PostgresRepository) GetProjectChatMessages(chatID int) ([]ProjectChatMessage, error) {
	rows, err := r.db.Query(
		`SELECT id, chat_id, role, content, created_at FROM project_chat_messages
		 WHERE chat_id = $1 ORDER BY created_at ASC`, chatID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []ProjectChatMessage
	for rows.Next() {
		var m ProjectChatMessage
		if err := rows.Scan(&m.ID, &m.ChatID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r *PostgresRepository) GetTaskSpecs(excludeTaskID int, limit int) ([]TaskSpecSummary, error) {
	rows, err := r.db.Query(
		`SELECT id, title, spec FROM tasks
		 WHERE spec != '' AND id != $1
		 ORDER BY id DESC LIMIT $2`,
		excludeTaskID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var specs []TaskSpecSummary
	for rows.Next() {
		var s TaskSpecSummary
		if err := rows.Scan(&s.ID, &s.Title, &s.Spec); err != nil {
			continue
		}
		specs = append(specs, s)
	}
	return specs, rows.Err()
}
