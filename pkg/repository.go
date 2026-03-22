package pkg

// Repository defines all database operations for the twist application.
type Repository interface {
	Ping() error
	Close() error

	// Tasks
	InsertTask(t Task) (int64, error)
	LoadTasks() ([]Task, error)
	GetTaskByID(id int) (Task, error)
	UpdateTaskStatus(id int, status string, approved bool) error
	UpdateTaskSpec(id int, spec string) error
	UpdateTaskBranch(id int, branch string) error
	UpdateTaskPRURL(id int, prURL string) error
	UpdateTaskSessionID(id int, sessionID string) error
	UpdateTaskChatSessionID(id int, chatSessionID string) error
	UpdateTaskWorktreePath(id int, path string) error
	UpdateTaskFields(id int, title, prompt, spec string) error
	DeleteTask(id int) error
	FindActionableTask() (Task, bool, error)
	FindActionableTasks() ([]Task, error)

	// Settings
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error

	// Design versions
	GetLatestDesignVersion() (int, string, error)
	InsertDesignVersion(version int, content string, taskID int, summary string) error
	GetDesignHistory() ([]DesignVersion, error)

	// Cross-task context
	GetTaskSpecs(excludeTaskID int, limit int) ([]TaskSpecSummary, error)

	// Chat messages
	InsertChatMessage(taskID int, role, content string) (ChatMessage, error)
	GetChatMessages(taskID int) ([]ChatMessage, error)

	// Task events
	InsertTaskEvent(taskID int, eventType, actor, summary, content string) error
	GetTaskEvents(taskID int) ([]TaskEvent, error)
}

// TaskSpecSummary is a lightweight view of a task for cross-task context injection.
type TaskSpecSummary struct {
	ID    int
	Title string
	Spec  string
}
