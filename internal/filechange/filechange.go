// Package filechange extracts file modifications from agent messages so
// that undo/redo can revert or re-apply them.
package filechange

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/message"
)

// Change describes the net effect that the file-modifying tool calls of
// an undone turn had on a single path.
type Change struct {
	Path        string
	OldContent  string
	NewContent  string
	FileExisted bool
}

// toolResultMeta is the union of the response metadata emitted by the
// file-modifying tools. Only the fields relevant to undo/redo are mapped.
type toolResultMeta struct {
	OldContent  string `json:"old_content"`
	NewContent  string `json:"new_content"`
	FileExisted bool   `json:"file_existed"`
}

// FromMessages extracts per-path file changes from the tool calls and
// results in the given messages. Multiple modifications of the same path
// are collapsed into a single change holding the earliest pre-image and
// the latest post-image. Unsuccessful tool calls are ignored since they
// did not touch the working tree.
func FromMessages(msgs []message.Message) []Change {
	type acc struct {
		oldContent  string
		newContent  string
		fileExisted bool
		hadOld      bool
	}
	order := []string{}
	byPath := make(map[string]*acc)

	pathsByCallID := make(map[string]string)
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			call, ok := part.(message.ToolCall)
			if !ok {
				continue
			}
			var input struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal([]byte(call.Input), &input); err != nil || input.FilePath == "" {
				continue
			}
			pathsByCallID[call.ID] = input.FilePath
		}
	}

	collect := func(path string, meta toolResultMeta, fileExisted bool) {
		if path == "" {
			return
		}
		path = resolvePath(path)
		a, ok := byPath[path]
		if !ok {
			a = &acc{}
			byPath[path] = a
			order = append(order, path)
		}
		if !a.hadOld {
			a.hadOld = true
			a.oldContent = meta.OldContent
			a.fileExisted = fileExisted
		}
		a.newContent = meta.NewContent
	}

	for _, msg := range msgs {
		for _, part := range msg.Parts {
			result, ok := part.(message.ToolResult)
			if !ok || result.IsError {
				continue
			}
			path := pathsByCallID[result.ToolCallID]
			var meta toolResultMeta
			switch result.Name {
			case tools.EditToolName, tools.MultiEditToolName, tools.ReplaceSymbolToolName:
				// Edit tools require an existing file, so the file always
				// existed regardless of the metadata contents.
				const fileExisted = true
				if path == "" && result.Name == tools.ReplaceSymbolToolName {
					var withPath struct {
						FilePath string `json:"file_path"`
					}
					if err := json.Unmarshal([]byte(result.Metadata), &withPath); err == nil {
						path = withPath.FilePath
					}
				}
				// Permission-denied results carry the proposed content but
				// never touched the tree; skip the empty no-op form.
				if err := json.Unmarshal([]byte(result.Metadata), &meta); err == nil &&
					(meta.OldContent != "" || meta.NewContent != "") {
					collect(path, meta, fileExisted)
				}
			case tools.WriteToolName:
				// A successful write always touches the working tree, even
				// when every field is empty: with file_existed false it
				// created a file, possibly an empty one.
				if err := json.Unmarshal([]byte(result.Metadata), &meta); err == nil {
					collect(path, meta, meta.FileExisted)
				}
			}
		}
	}

	changes := make([]Change, 0, len(order))
	for _, path := range order {
		a := byPath[path]
		changes = append(changes, Change{
			Path:        path,
			OldContent:  a.oldContent,
			NewContent:  a.newContent,
			FileExisted: a.fileExisted,
		})
	}
	return changes
}

// resolvePath resolves relative tool paths against the current working
// directory, mirroring how the tools resolve their inputs. The backend
// runs in the workspace directory, so relative paths resolve correctly.
func resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, path)
	}
	return path
}

// Revert rolls the working tree back to the state captured by the given
// changes. Files created during the undone turn are removed.
func Revert(changes []Change) error {
	for _, c := range changes {
		if !c.FileExisted {
			if err := os.Remove(c.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("Failed to remove %s: %w", c.Path, err)
			}
			continue
		}
		if err := os.WriteFile(c.Path, []byte(c.OldContent), 0o644); err != nil {
			return fmt.Errorf("Failed to restore %s: %w", c.Path, err)
		}
	}
	return nil
}

// Apply re-applies the file modifications captured by the given changes,
// used to redo an undo.
func Apply(changes []Change) error {
	for _, c := range changes {
		if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
			return fmt.Errorf("Failed to create directory for %s: %w", c.Path, err)
		}
		if err := os.WriteFile(c.Path, []byte(c.NewContent), 0o644); err != nil {
			return fmt.Errorf("Failed to restore %s: %w", c.Path, err)
		}
	}
	return nil
}
