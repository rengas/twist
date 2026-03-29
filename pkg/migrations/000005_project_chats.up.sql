CREATE TABLE IF NOT EXISTS project_chats (
    id         SERIAL PRIMARY KEY,
    session_id TEXT NOT NULL DEFAULT '',
    title      TEXT NOT NULL DEFAULT 'New Chat',
    archived   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS project_chat_messages (
    id         SERIAL PRIMARY KEY,
    chat_id    INTEGER NOT NULL REFERENCES project_chats(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content    TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_project_chat_messages_chat_id ON project_chat_messages(chat_id);
