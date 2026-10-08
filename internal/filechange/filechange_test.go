package filechange

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

func msgWithParts(parts ...message.ContentPart) message.Message {
	return message.Message{ID: "msg", Parts: parts}
}

func toolCallMsg(id, name, filePath string) message.Message {
	return msgWithParts(message.ToolCall{
		ID:    id,
		Name:  name,
		Input: `{"file_path":"` + filePath + `"}`,
	})
}

func toolResultMsg(callID, name, metadata string) message.Message {
	return msgWithParts(message.ToolResult{
		ToolCallID: callID,
		Name:       name,
		Metadata:   metadata,
	})
}

func TestFromMessagesCollapsesMultipleEdits(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		toolCallMsg("c1", tools.EditToolName, "/tmp/a.go"),
		toolCallMsg("c2", tools.WriteToolName, "/tmp/a.go"),
		toolCallMsg("c3", tools.WriteToolName, "/tmp/b.go"),
		toolResultMsg("c1", tools.EditToolName,
			`{"old_content":"one","new_content":"two"}`),
		toolResultMsg("c2", tools.WriteToolName,
			`{"old_content":"two","new_content":"three","file_existed":true}`),
		toolResultMsg("c3", tools.WriteToolName,
			`{"new_content":"new file"}`),
	}

	changes := FromMessages(msgs)
	require.Len(t, changes, 2)

	require.Equal(t, "/tmp/a.go", changes[0].Path)
	require.Equal(t, "one", changes[0].OldContent)
	require.Equal(t, "three", changes[0].NewContent)
	require.True(t, changes[0].FileExisted)

	require.Equal(t, "/tmp/b.go", changes[1].Path)
	require.Equal(t, "", changes[1].OldContent)
	require.Equal(t, "new file", changes[1].NewContent)
	require.False(t, changes[1].FileExisted)
}

func TestFromMessagesIgnoresErrorsAndNonFileTools(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		toolCallMsg("c1", "bash", "irrelevant"),
		toolResultMsg("c1", "bash", `{"cmd":"echo hi"}`),
		toolCallMsg("c2", tools.EditToolName, "/tmp/x.go"),
	}
	result := toolResultMsg("c2", tools.EditToolName,
		`{"old_content":"a","new_content":"b"}`)
	failed := result.Parts[0].(message.ToolResult)
	failed.IsError = true
	result.Parts[0] = failed
	msgs = append(msgs, result)

	require.Empty(t, FromMessages(msgs))
}

func TestFromMessagesWriteWithoutContents(t *testing.T) {
	t.Parallel()

	// Regression: a write that created a file stores no old_content (file
	// did not exist) and older binaries also omitted new_content. It must
	// still be treated as a change so undo removes the file.
	msgs := []message.Message{
		toolCallMsg("c1", tools.WriteToolName, "/tmp/new.txt"),
		toolResultMsg("c1", tools.WriteToolName,
			`{"diff":"--- a/new.txt","additions":1,"removals":0,"file_existed":false}`),
	}

	changes := FromMessages(msgs)
	require.Len(t, changes, 1)
	require.Equal(t, "/tmp/new.txt", changes[0].Path)
	require.False(t, changes[0].FileExisted)
}

func TestRevertAndApply(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.txt")
	created := filepath.Join(dir, "created", "new.txt")

	require.NoError(t, os.WriteFile(existing, []byte("original"), 0o644))

	changes := []Change{
		{Path: existing, OldContent: "original", NewContent: "edited", FileExisted: true},
		{Path: created, OldContent: "", NewContent: "created content", FileExisted: false},
	}

	// Simulate the agent's turn: existing edited, new file created.
	require.NoError(t, os.WriteFile(existing, []byte("edited"), 0o644))
	require.NoError(t, Apply(nil)) // no-op
	require.NoError(t, os.MkdirAll(filepath.Dir(created), 0o755))
	require.NoError(t, os.WriteFile(created, []byte("created content"), 0o644))

	// Undo: edits reverted, created file removed.
	require.NoError(t, Revert(changes))
	restored, err := os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "original", string(restored))
	_, err = os.Stat(created)
	require.True(t, os.IsNotExist(err))

	// Redo: changes re-applied.
	require.NoError(t, Apply(changes))
	redone, err := os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "edited", string(redone))
	createdContent, err := os.ReadFile(created)
	require.NoError(t, err)
	require.Equal(t, "created content", string(createdContent))
}
