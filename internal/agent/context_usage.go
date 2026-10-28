package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/message"
)

// ContextUsage is an estimated breakdown of the tokens that fill the
// model's context window for a session, grouped by origin: the
// conversation transcript (user, agent, tools, system messages) and the
// static scaffolding sent with every request (system prompt, tools,
// skills, sub-agent definitions).
//
// All values are approximations derived from content sizes, not provider
// token counts. They are meant for the context usage dialog, not for
// billing or compaction decisions.
type ContextUsage struct {
	UserMessages   int64 `json:"user_messages"`
	AgentResponses int64 `json:"agent_responses"`
	ToolCalls      int64 `json:"tool_calls"`
	SystemMessages int64 `json:"system_messages"`
	SystemPrompt   int64 `json:"system_prompt"`
	SystemTools    int64 `json:"system_tools"`
	Skills         int64 `json:"skills"`
	Subagents      int64 `json:"subagents"`
}

// Total returns the sum of every category.
func (u ContextUsage) Total() int64 {
	return u.UserMessages + u.AgentResponses + u.ToolCalls + u.SystemMessages +
		u.SystemPrompt + u.SystemTools + u.Skills + u.Subagents
}

// The skills XML block is embedded in the rendered system prompt by
// prompt.Build. It is carved out so the dialog can attribute those
// tokens to skills without double counting them as system prompt.
const (
	skillsXMLStart = "<available_skills>"
	skillsXMLEnd   = "</available_skills>"
)

// ContextUsage estimates how the session's context window is filled. The
// transcript part is derived from the messages a run would actually send
// (the tail kept after compaction), the scaffolding part from the
// agent's current system prompt and tool set.
func (a *sessionAgent) ContextUsage(ctx context.Context, sessionID string) (ContextUsage, error) {
	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return ContextUsage{}, fmt.Errorf("failed to get session: %w", err)
	}
	msgs, err := a.getSessionMessages(ctx, sess)
	if err != nil {
		return ContextUsage{}, err
	}

	usage := estimateMessageContextUsage(msgs)
	usage.SystemPrompt, usage.Skills = splitPromptTokens(a.systemPrompt.Get())

	for _, tool := range a.tools.Copy() {
		info := tool.Info()
		if info.Name == "" && info.Description == "" {
			continue
		}
		tokens := toolInfoTokens(info)
		if info.Name == AgentToolName {
			usage.Subagents += tokens
		} else {
			usage.SystemTools += tokens
		}
	}
	return usage, nil
}

// splitPromptTokens splits a rendered system prompt into the skills XML
// block and everything else.
func splitPromptTokens(prompt string) (systemPrompt, skills int64) {
	if prompt == "" {
		return 0, 0
	}
	start := strings.Index(prompt, skillsXMLStart)
	if start == -1 {
		return approxTokenCount(prompt), 0
	}
	end := strings.Index(prompt[start:], skillsXMLEnd)
	if end == -1 {
		return approxTokenCount(prompt), 0
	}
	end += start + len(skillsXMLEnd)
	rest := prompt[:start] + prompt[end:]
	return approxTokenCount(rest), approxTokenCount(prompt[start:end])
}

// toolInfoTokens estimates the tokens a tool definition costs in the
// request: its name, description, and JSON schema.
func toolInfoTokens(info fantasy.ToolInfo) int64 {
	tokens := approxTokenCount(info.Name) + approxTokenCount(info.Description)
	if params, err := json.Marshal(info.Parameters); err == nil {
		tokens += approxTokenCount(string(params))
	}
	if required, err := json.Marshal(info.Required); err == nil {
		tokens += approxTokenCount(string(required))
	}
	return tokens
}

// estimateMessageContextUsage estimates transcript tokens by message
// role and part type, mirroring the role the part plays in the request.
func estimateMessageContextUsage(msgs []message.Message) ContextUsage {
	var usage ContextUsage
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			usage.addPart(msg.Role, part)
		}
	}
	return usage
}

func (u *ContextUsage) addPart(role message.MessageRole, part message.ContentPart) {
	switch p := part.(type) {
	case message.TextContent:
		switch role {
		case message.Assistant:
			u.AgentResponses += approxTokenCount(p.Text)
		case message.System:
			u.SystemMessages += approxTokenCount(p.Text)
		default:
			u.UserMessages += approxTokenCount(p.Text)
		}
	case message.ReasoningContent:
		u.AgentResponses += approxTokenCount(p.Thinking)
	case message.ToolCall:
		u.ToolCalls += approxTokenCount(p.Name) + approxTokenCount(p.Input)
	case message.ToolResult:
		u.ToolCalls += toolResultTokens(p)
	case message.BinaryContent:
		u.UserMessages += approxTokenCount(fmt.Sprintf("%s %s %d bytes", p.MIMEType, p.Path, len(p.Data)))
	case message.ShellCommand:
		u.UserMessages += approxTokenCount(p.Command) + approxTokenCount(p.Output)
	}
}

// toolResultTokens estimates a stored tool result. Only the output
// content (and media data) is sent to the provider — ToAIMessage drops
// the result's Name and Metadata (edit results store full file diffs
// in Metadata purely for the session's undo history) — so those are
// not counted.
func toolResultTokens(result message.ToolResult) int64 {
	if result.Data != "" {
		// Binary payloads travel as media; count by size, like the
		// usage fallback does.
		return approxTokenCount(fmt.Sprintf("%s %d bytes", result.MIMEType, len(result.Data)))
	}
	return approxTokenCount(result.Content)
}
