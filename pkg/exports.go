package pkg

import "database/sql"

// Exported wrappers for functions that remain unexported in kanban.go.

func DeleteTask(db *sql.DB, id int) error { return deleteTask(db, id) }
func Slugify(s string) string             { return slugify(s) }