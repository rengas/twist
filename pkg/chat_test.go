package pkg

import (
	"testing"
)

func TestParseStreamLine_InitMessage(t *testing.T) {
	line := `{"type":"init","session_id":"abc-123"}`
	sid, content, result, ok := parseStreamLine(line)
	if !ok {
		t.Fatal("expected ok=true for valid JSON")
	}
	if sid != "abc-123" {
		t.Errorf("session_id: got %q, want %q", sid, "abc-123")
	}
	if content != "" {
		t.Errorf("content: got %q, want empty", content)
	}
	if result != "" {
		t.Errorf("result: got %q, want empty", result)
	}
}

func TestParseStreamLine_ContentMessage(t *testing.T) {
	line := `{"type":"assistant","content":"Hello world"}`
	sid, content, result, ok := parseStreamLine(line)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if sid != "" {
		t.Errorf("session_id: got %q, want empty", sid)
	}
	if content != "Hello world" {
		t.Errorf("content: got %q, want %q", content, "Hello world")
	}
	if result != "" {
		t.Errorf("result: got %q, want empty", result)
	}
}

func TestParseStreamLine_ResultMessage(t *testing.T) {
	line := `{"type":"result","result":"Full response text","session_id":"abc-123"}`
	sid, content, result, ok := parseStreamLine(line)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if sid != "abc-123" {
		t.Errorf("session_id: got %q, want %q", sid, "abc-123")
	}
	if result != "Full response text" {
		t.Errorf("result: got %q, want %q", result, "Full response text")
	}
	if content != "" {
		t.Errorf("content: got %q, want empty", content)
	}
}

func TestParseStreamLine_InvalidJSON(t *testing.T) {
	line := "this is not json"
	_, _, _, ok := parseStreamLine(line)
	if ok {
		t.Error("expected ok=false for invalid JSON")
	}
}

func TestParseStreamLine_EmptyObject(t *testing.T) {
	line := `{}`
	sid, content, result, ok := parseStreamLine(line)
	if !ok {
		t.Fatal("expected ok=true for valid JSON")
	}
	if sid != "" || content != "" || result != "" {
		t.Errorf("expected all empty, got sid=%q content=%q result=%q", sid, content, result)
	}
}

func requireArgsEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d\nargs: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildChatArgs(t *testing.T) {
	tests := []struct {
		name     string
		opts     chatInvokeOpts
		expected []string
	}{
		{
			name:     "ResumeWithFork",
			opts:     chatInvokeOpts{ResumeSessionID: "sess-123", Fork: true, Message: "hello"},
			expected: []string{"-p", "--dangerously-skip-permissions", "--verbose", "--output-format", "stream-json", "--resume", "sess-123", "--fork-session", "hello"},
		},
		{
			name:     "ResumeWithoutFork",
			opts:     chatInvokeOpts{ResumeSessionID: "sess-123", Fork: false, Message: "hello"},
			expected: []string{"-p", "--dangerously-skip-permissions", "--verbose", "--output-format", "stream-json", "--resume", "sess-123", "hello"},
		},
		{
			name:     "SessionID",
			opts:     chatInvokeOpts{SessionID: "fresh-uuid", Message: "hello"},
			expected: []string{"-p", "--dangerously-skip-permissions", "--verbose", "--output-format", "stream-json", "--session-id", "fresh-uuid", "hello"},
		},
		{
			name:     "NoSession",
			opts:     chatInvokeOpts{Message: "hello"},
			expected: []string{"-p", "--dangerously-skip-permissions", "--verbose", "--output-format", "stream-json", "hello"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildChatArgs(tt.opts)
			requireArgsEqual(t, args, tt.expected)
		})
	}
}

func TestBuildChatArgs_ResumePreferredOverSessionID(t *testing.T) {
	// When both ResumeSessionID and SessionID are set, Resume takes precedence.
	args := buildChatArgs(chatInvokeOpts{
		ResumeSessionID: "resume-id",
		SessionID:       "session-id",
		Message:         "hello",
	})
	// Should use --resume, not --session-id.
	found := false
	for _, a := range args {
		if a == "--resume" {
			found = true
		}
		if a == "--session-id" {
			t.Error("--session-id should not appear when ResumeSessionID is set")
		}
	}
	if !found {
		t.Error("expected --resume in args")
	}
}
