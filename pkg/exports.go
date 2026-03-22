package pkg

import "sync"

// Exported wrappers so that the test package (package main) can call them via pkg.FuncName.

func Slugify(s string) string                    { return slugify(s) }
func GenerateUUID() string                       { return generateUUID() }
func Truncate(s string, max int) string          { return truncate(s, max) }
func ReadDesignContent(workDir string) string     { return readDesignContent(workDir) }
func BuildTaskContext(repo Repository, id int) string { return buildTaskContext(repo, id) }
func AppendDesignVersion(repo Repository, mu *sync.Mutex, workDir string, taskID int, section, summary string) {
	appendDesignVersion(repo, mu, workDir, taskID, section, summary)
}
func CreateWorktree(workDir, branch string, taskID int) (string, error) {
	return createWorktree(workDir, branch, taskID)
}
func RemoveWorktree(workDir, wtPath string) error { return removeWorktree(workDir, wtPath) }
func BuildChatContextMessage(title, spec, userMessage string) string {
	return buildChatContextMessage(title, spec, userMessage)
}
func ParseStreamLine(line string) (sessionID, content, result string, ok bool) {
	return parseStreamLine(line)
}
func BuildChatArgs(opts chatInvokeOpts) []string { return buildChatArgs(opts) }

// SetRepoForTest injects a repository for testing without a full Wails context.
func (a *App) SetRepoForTest(repo Repository) {
	a.repo = repo
}

// Export types for testing.
type ChatInvokeOpts = chatInvokeOpts
type ChatInvokeResult = chatInvokeResult
type StreamLine = streamLine
