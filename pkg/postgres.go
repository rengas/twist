package pkg

import (
	"database/sql"
	"fmt"

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
	r.db.Exec(`TRUNCATE tasks, settings, design_versions RESTART IDENTITY`)
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

const pgTaskColumns = `id, title, prompt, spec, branch, pr_url, status, approved, session_id, worktree_path`

func scanTaskRow(scanner interface {
	Scan(dest ...any) error
}) (Task, error) {
	var t Task
	err := scanner.Scan(
		&t.ID, &t.Title, &t.Prompt, &t.Spec, &t.Branch, &t.PRURL,
		&t.Status, &t.Approved, &t.SessionID, &t.WorktreePath,
	)
	return t, err
}

func (r *PostgresRepository) InsertTask(t Task) (int64, error) {
	var id int64
	err := r.db.QueryRow(
		`INSERT INTO tasks (title, prompt, spec, branch, pr_url, status, approved, session_id, worktree_path)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		t.Title, t.Prompt, t.Spec, t.Branch, t.PRURL, t.Status, t.Approved, t.SessionID, t.WorktreePath,
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

func (r *PostgresRepository) UpdateTaskWorktreePath(id int, path string) error {
	_, err := r.db.Exec(`UPDATE tasks SET worktree_path=$1 WHERE id=$2`, path, id)
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
