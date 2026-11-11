package agent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"

	"charm.land/fantasy"
	"golang.org/x/sync/errgroup"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
)

//go:embed templates/agent_tool.md.tpl
var agentToolDescriptionTmpl string

var agentToolDescriptionTemplate = template.Must(template.New("agent_tool").Parse(agentToolDescriptionTmpl))

// agentToolTemplateData carries the sub-agent model's capabilities into the
// tool description, so the parent model only learns about reasoning_effort
// and image viewing when the selected model actually supports them.
type agentToolTemplateData struct {
	CanReason       bool
	HasVision       bool
	ReasoningLevels string
}

// agentToolDescriptionFor renders the tool description for the given model.
func agentToolDescriptionFor(model Model) (string, error) {
	var sb strings.Builder
	data := agentToolTemplateData{
		CanReason:       model.CatwalkCfg.CanReason(),
		HasVision:       model.CatwalkCfg.Capabilities.Vision,
		ReasoningLevels: strings.Join(model.CatwalkCfg.ReasoningEffortLevels(), ", "),
	}
	if err := agentToolDescriptionTemplate.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("rendering agent tool description: %w", err)
	}
	return sb.String(), nil
}

type AgentParams struct {
	Prompt string `json:"prompt" description:"The task for the agent to perform"`
	// ReasoningEffort tunes the sub-agent model's thinking for this task:
	// "off" for simple work like summarization, a level name such as
	// "low", "medium", or "high" (whatever the model supports), or "max"
	// for the highest supported level when hard reasoning is needed.
	ReasoningEffort string `json:"reasoning_effort,omitempty" description:"Optional reasoning effort for the sub-agent model: \"off\" to disable thinking, a supported level name (e.g. low, medium, high), or \"max\" for the highest level. Use low/off for trivial tasks like summarization and max for hard inference. Omit to use the model default."`
	Access          string `json:"access,omitempty" description:"\"read\" (default) for a subagent that can only inspect the codebase, or \"write\" for one that can also edit files"`
}

const (
	AgentToolName = "agent"

	accessRead  = "read"
	accessWrite = "write"
)

// writeAgentConfig derives the read-write variant of the task agent: the same
// agent, given the coder's tools. Subagents never get the delegation tools;
// effectiveToolNames strips them.
func writeAgentConfig(cfg *config.Config) config.Agent {
	task := cfg.Agents[config.AgentTask]
	if coder, ok := cfg.Agents[config.AgentCoder]; ok {
		task.AllowedTools = slices.Clone(coder.AllowedTools)
	}
	return task
}

// accessOrRead defaults an unset access level to read-only.
func accessOrRead(access string) string {
	if access == "" {
		return accessRead
	}
	return access
}

// readOnlyParent reports whether a parent agent may only delegate to
// read-only subagents. Plan mode promises it cannot change files, and that
// promise has to cover the work it hands off as well as the work it does
// itself.
func readOnlyParent(parentAgentID string) bool {
	return parentAgentID == config.AgentPlan
}

// resolveSubAgent picks the subagent variant for a requested access level,
// returning a message for the model when there is no such variant.
func resolveSubAgent(agents map[string]SessionAgent, access string, readOnly bool) (SessionAgent, string) {
	access = accessOrRead(access)
	if agent, ok := agents[access]; ok {
		return agent, ""
	}
	if readOnly && access == accessWrite {
		return nil, `plan mode subagents are read-only; omit access or pass "read"`
	}
	return nil, `access must be "read" or "write"`
}

// taskAgents builds the variants of the task subagent. Both the synchronous
// agent tool and the detached dispatch tool draw from these. A read-only
// parent gets only the read variant, so the write one is never built rather
// than built and withheld.
func (c *coordinator) taskAgents(ctx context.Context, readOnly bool) (map[string]SessionAgent, error) {
	if _, ok := c.cfg.Config().Agents[config.AgentTask]; !ok {
		return nil, errors.New("task agent not configured")
	}
	accesses := []string{accessRead, accessWrite}
	if readOnly {
		accesses = accesses[:1]
	}

	// The variants share a readiness group of their own and are fully set
	// up before this returns. This runs on every turn, concurrently with
	// other runs waiting on the coordinator's group, so joining that group
	// would race its Wait.
	var ready errgroup.Group
	agents := map[string]SessionAgent{}
	for _, access := range accesses {
		agentCfg, canWrite := c.taskAgentConfig(access)
		agent, err := c.buildTaskAgent(ctx, &ready, agentCfg, canWrite, nil)
		if err != nil {
			return nil, err
		}
		agents[access] = agent
	}
	if err := ready.Wait(); err != nil {
		return nil, err
	}
	return agents, nil
}

// taskAgentConfig returns the task agent's config for an access level, and
// whether it may write.
func (c *coordinator) taskAgentConfig(access string) (config.Agent, bool) {
	if access == accessWrite {
		return writeAgentConfig(c.cfg.Config()), true
	}
	return c.cfg.Config().Agents[config.AgentTask], false
}

// buildTaskAgent schedules one variant of the task subagent into ready; the
// caller waits on the group, so several variants can build concurrently.
// modelsFn, when set, replaces the shared subagent model set; a dispatch
// with a model override uses it to run the variant on the overridden model.
func (c *coordinator) buildTaskAgent(ctx context.Context, ready *errgroup.Group, agentCfg config.Agent, canWrite bool, modelsFn func() ModelSet) (SessionAgent, error) {
	pr, err := taskPrompt(
		prompt.WithWorkingDir(c.cfg.WorkingDir()),
		prompt.WithCanWrite(canWrite),
	)
	if err != nil {
		return nil, err
	}
	return c.buildAgentWithModels(ctx, ready, pr, agentCfg, true, modelsFn)
}

func (c *coordinator) agentTool(ctx context.Context, parentAgentID string) (fantasy.AgentTool, error) {
	readOnly := readOnlyParent(parentAgentID)
	agents, err := c.taskAgents(ctx, readOnly)
	if err != nil {
		return nil, err
	}
	description, err := agentToolDescriptionFor(agents[accessRead].Model())
	if err != nil {
		return nil, err
	}
	return fantasy.NewParallelAgentTool(
		AgentToolName,
		description,
		func(ctx context.Context, params AgentParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Prompt == "" {
				return fantasy.NewTextErrorResponse("prompt is required"), nil
			}

			agent, refusal := resolveSubAgent(agents, params.Access, readOnly)
			if refusal != "" {
				return fantasy.NewTextErrorResponse(refusal), nil
			}

			sessionID := tools.GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, errors.New("session id missing from context")
			}

			agentMessageID := tools.GetMessageFromContext(ctx)
			if agentMessageID == "" {
				return fantasy.ToolResponse{}, errors.New("agent message id missing from context")
			}

			return c.runSubAgentMaybeBackground(ctx, AgentToolName, params.Access, subAgentParams{
				Agent:           agent,
				SessionID:       sessionID,
				AgentMessageID:  agentMessageID,
				ToolCallID:      call.ID,
				Prompt:          params.Prompt,
				ReasoningEffort: params.ReasoningEffort,
				SessionTitle:    "New Agent Session",
			})
		},
	), nil
}
