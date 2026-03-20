package pkg

import (
	"database/sql"
	"sync"
)

// Exported wrappers for functions that remain unexported in kanban.go.
// These are needed so that the test package (package main) can call them via pkg.FuncName.

func InsertTask(db *sql.DB, t Task) (int64, error)                { return insertTask(db, t) }
func LoadTasks(db *sql.DB) ([]Task, error)                        { return loadTasks(db) }
func UpdateTaskStatus(db *sql.DB, id int, s string, a bool) error { return updateTaskStatus(db, id, s, a) }
func UpdateTaskSpec(db *sql.DB, id int, spec string) error        { return updateTaskSpec(db, id, spec) }
func UpdateTaskBranch(db *sql.DB, id int, branch string) error    { return updateTaskBranch(db, id, branch) }
func UpdateTaskPRURL(db *sql.DB, id int, prURL string) error      { return updateTaskPRURL(db, id, prURL) }
func UpdateTaskSessionID(db *sql.DB, id int, sid string) error    { return updateTaskSessionID(db, id, sid) }
func UpdateTaskWorktreePath(db *sql.DB, id int, p string) error   { return updateTaskWorktreePath(db, id, p) }
func DeleteTask(db *sql.DB, id int) error                         { return deleteTask(db, id) }
func GetTaskByID(db *sql.DB, id int) (Task, error)                { return getTaskByID(db, id) }
func FindActionableDB(db *sql.DB) (Task, bool, error)             { return findActionableDB(db) }
func FindActionableTasksDB(db *sql.DB) ([]Task, error)            { return findActionableTasksDB(db) }
func BoolToInt(b bool) int                                        { return boolToInt(b) }
func Slugify(s string) string                                     { return slugify(s) }
func ScanTask(row *sql.Row) (Task, error)                         { return scanTask(row) }
func GenerateUUID() string                                        { return generateUUID() }
func Truncate(s string, max int) string                           { return truncate(s, max) }
func BuildTaskContext(db *sql.DB, excludeID int) (string, error)  { return buildTaskContext(db, excludeID) }
func GetLatestDesignVersion(db *sql.DB) (int, string, error)      { return getLatestDesignVersion(db) }
func GetDesignHistory(db *sql.DB) ([]DesignVersion, error)        { return getDesignHistory(db) }
func AppendDesignVersion(db *sql.DB, mu *sync.Mutex, workDir string, taskID int, section, summary string) {
	appendDesignVersion(db, mu, workDir, taskID, section, summary)
}
func CreateWorktree(workDir, branch string, taskID int) (string, error) {
	return createWorktree(workDir, branch, taskID)
}
func RemoveWorktree(workDir, wtPath string) error { return removeWorktree(workDir, wtPath) }
