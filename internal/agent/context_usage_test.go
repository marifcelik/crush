package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/message"
)

func TestEstimateMessageContextUsage(t *testing.T) {
	userText := strings.Repeat("a", 40)
	agentText := strings.Repeat("b", 60)
	thinking := strings.Repeat("c", 20)
	toolInput := `{"command":"ls"}`
	resultContent := "file1\nfile2"
	systemText := strings.Repeat("e", 30)

	msgs := []message.Message{
		{
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: userText},
				message.ShellCommand{Command: "ls", Output: resultContent},
			},
		},
		{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: agentText},
				message.ReasoningContent{Thinking: thinking},
				message.ToolCall{Name: "bash", Input: toolInput},
			},
		},
		{
			Role: message.User,
			Parts: []message.ContentPart{
				message.ToolResult{
					Name:     "bash",
					Content:  resultContent,
					Metadata: `{"old_content":"huge diff","new_content":"huge diff"}`,
				},
			},
		},
		{
			Role:  message.System,
			Parts: []message.ContentPart{message.TextContent{Text: systemText}},
		},
	}

	usage := estimateMessageContextUsage(msgs)

	expectedUser := approxTokenCount(userText) +
		approxTokenCount("ls") + approxTokenCount(resultContent)
	expectedAgent := approxTokenCount(agentText) + approxTokenCount(thinking)
	// Only what ToAIMessage sends is counted: the result's text output.
	// Name and Metadata (edit diffs) never reach the provider.
	expectedTools := approxTokenCount("bash") + approxTokenCount(toolInput) +
		approxTokenCount(resultContent)
	expectedSystem := approxTokenCount(systemText)

	require.Equal(t, expectedUser, usage.UserMessages)
	require.Equal(t, expectedAgent, usage.AgentResponses)
	require.Equal(t, expectedTools, usage.ToolCalls)
	require.Equal(t, expectedSystem, usage.SystemMessages)
	require.Equal(t, expectedUser+expectedAgent+expectedTools+expectedSystem, usage.Total())
	require.Zero(t, usage.SystemPrompt)
	require.Zero(t, usage.Skills)
}

func TestSplitPromptTokens(t *testing.T) {
	head := strings.Repeat("x", 400)
	skills := "<available_skills>\n<skill>noop</skill>\n</available_skills>"
	tail := strings.Repeat("y", 400)

	withoutSkills, skillsTokens := splitPromptTokens(head + tail)
	require.Equal(t, approxTokenCount(head+tail), withoutSkills)
	require.Zero(t, skillsTokens)

	prompt, skillsTokens := splitPromptTokens(head + skills + tail)
	require.Equal(t, approxTokenCount(skills), skillsTokens)
	require.Equal(t, approxTokenCount(head+tail), prompt)

	// An unterminated block falls back to counting the whole prompt.
	prompt, skillsTokens = splitPromptTokens(head + "<available_skills>")
	require.Equal(t, approxTokenCount(head+"<available_skills>"), prompt)
	require.Zero(t, skillsTokens)
}

func TestToolResultMediaTokens(t *testing.T) {
	result := message.ToolResult{
		Name:     "view",
		Content:  "unused when data is set",
		Data:     strings.Repeat("d", 100),
		MIMEType: "image/png",
	}
	// Media payloads are counted by size, not by the base64 content.
	require.Equal(t, approxTokenCount("image/png 100 bytes"), toolResultTokens(result))
}

func TestToolInfoTokens(t *testing.T) {
	tokens := toolInfoTokens(fantasy.ToolInfo{
		Name:        "bash",
		Description: "Run a shell command",
		Parameters:  map[string]any{"command": map[string]any{"type": "string"}},
		Required:    []string{"command"},
	})
	require.Greater(t, tokens, int64(0))
	require.Less(t, tokens, int64(50))
}
