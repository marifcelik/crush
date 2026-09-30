package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/stretchr/testify/require"
)

func namedTool(name string) fantasy.AgentTool {
	return &fakeTool{name: name}
}

func TestRepairToolCallAliasRewritesReadToView(t *testing.T) {
	t.Parallel()

	available := []fantasy.AgentTool{namedTool(tools.ViewToolName)}

	repaired, err := repairToolCallAlias(context.Background(), fantasy.ToolCallRepairOptions{
		OriginalToolCall: fantasy.ToolCallContent{
			ToolName: readToolCallName,
			Input:    `{"file_path":"main.go"}`,
		},
		AvailableTools: available,
	})
	require.NoError(t, err)
	require.NotNil(t, repaired)
	require.Equal(t, tools.ViewToolName, repaired.ToolName)
	require.JSONEq(t, `{"file_path":"main.go"}`, repaired.Input)
}

func TestRepairToolCallAliasNormalizesParams(t *testing.T) {
	t.Parallel()

	available := []fantasy.AgentTool{namedTool(tools.ViewToolName)}

	repaired, err := repairToolCallAlias(context.Background(), fantasy.ToolCallRepairOptions{
		OriginalToolCall: fantasy.ToolCallContent{
			ToolName: readToolCallName,
			Input:    `{"path":"main.go","start_line":10,"lines":50}`,
		},
		AvailableTools: available,
	})
	require.NoError(t, err)
	require.NotNil(t, repaired)
	require.Equal(t, tools.ViewToolName, repaired.ToolName)
	require.JSONEq(t, `{"file_path":"main.go","offset":10,"limit":50}`, repaired.Input)
}

func TestRepairToolCallAliasKeepsCanonicalParams(t *testing.T) {
	t.Parallel()

	available := []fantasy.AgentTool{namedTool(tools.ViewToolName)}

	repaired, err := repairToolCallAlias(context.Background(), fantasy.ToolCallRepairOptions{
		OriginalToolCall: fantasy.ToolCallContent{
			ToolName: readToolCallName,
			Input:    `{"path":"main.go","file_path":"other.go"}`,
		},
		AvailableTools: available,
	})
	require.NoError(t, err)
	require.NotNil(t, repaired)
	require.JSONEq(t, `{"file_path":"other.go"}`, repaired.Input)
}

func TestRepairToolCallAliasSkipsWhenViewUnavailable(t *testing.T) {
	t.Parallel()

	// view is disabled for this step: rewriting would fail validation anyway.
	repaired, err := repairToolCallAlias(context.Background(), fantasy.ToolCallRepairOptions{
		OriginalToolCall: fantasy.ToolCallContent{
			ToolName: readToolCallName,
			Input:    `{"file_path":"main.go"}`,
		},
		AvailableTools: []fantasy.AgentTool{namedTool(tools.GrepToolName)},
	})
	require.NoError(t, err)
	require.Nil(t, repaired)
}

func TestRepairToolCallAliasPreservesOtherCalls(t *testing.T) {
	t.Parallel()

	// A tool that exists must never reach the repair function; this guards
	// the pass-through shape anyway.
	repaired, err := repairToolCallAlias(context.Background(), fantasy.ToolCallRepairOptions{
		OriginalToolCall: fantasy.ToolCallContent{
			ToolName: tools.GrepToolName,
			Input:    `{"pattern":"foo"}`,
		},
		AvailableTools: []fantasy.AgentTool{namedTool(tools.GrepToolName)},
	})
	require.NoError(t, err)
	require.Nil(t, repaired)
}

func TestRepairToolCallAliasRepairsMalformedJSON(t *testing.T) {
	t.Parallel()

	available := []fantasy.AgentTool{namedTool(tools.ViewToolName)}

	repaired, err := repairToolCallAlias(context.Background(), fantasy.ToolCallRepairOptions{
		OriginalToolCall: fantasy.ToolCallContent{
			ToolName: readToolCallName,
			Input:    `{"path": "main.go",}`,
		},
		AvailableTools: available,
	})
	require.NoError(t, err)
	require.NotNil(t, repaired)
	require.Equal(t, tools.ViewToolName, repaired.ToolName)
	require.JSONEq(t, `{"file_path":"main.go"}`, repaired.Input)
}
