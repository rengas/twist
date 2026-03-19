package pkg

import "database/sql"

// Exported wrappers for testing from the root package.

func InsertTask(db *sql.DB, t Task) (int64, error)                { return insertTask(db, t) }
func LoadTasks(db *sql.DB) ([]Task, error)                        { return loadTasks(db) }
func UpdateTaskStatus(db *sql.DB, id int, s string, a bool) error { return updateTaskStatus(db, id, s, a) }
func UpdateTaskSpec(db *sql.DB, id int, spec string) error        { return updateTaskSpec(db, id, spec) }
func UpdateTaskBranch(db *sql.DB, id int, branch string) error    { return updateTaskBranch(db, id, branch) }
func UpdateTaskPRURL(db *sql.DB, id int, prURL string) error      { return updateTaskPRURL(db, id, prURL) }
func DeleteTask(db *sql.DB, id int) error                         { return deleteTask(db, id) }
func FindActionableDB(db *sql.DB) (Task, bool, error)             { return findActionableDB(db) }
func BoolToInt(b bool) int                                        { return boolToInt(b) }
func Slugify(s string) string                                     { return slugify(s) }
func MigrateFromJSON(dir string, db *sql.DB)                      { migrateFromJSON(dir, db) }
