package agent

import (
	"context"
	"encoding/json"
	"slices"

	"charm.land/fantasy"
	"charm.land/fantasy/jsonrepair"
	"github.com/charmbracelet/crush/internal/agent/tools"
)

// readToolCallName is the tool name models reach for instead of view. It
// never exists in the tool list, so the call would otherwise fail
// validation with "tool not found: read".
const readToolCallName = "read"

// viewParamAliases maps the parameter names models pair with a "read" call
// onto the names view expects. Anything not listed is left untouched.
var viewParamAliases = map[string]string{
	"path":       "file_path",
	"filePath":   "file_path",
	"start_line": "offset",
	"line_count": "limit",
	"lines":      "limit",
}

// repairToolCallAlias repairs tool calls that fail validation. It rewrites
// a "read" call into the equivalent "view" call — same arguments, translated
// to view's parameter names — so the model gets the file contents it asked
// for instead of a "tool not found" error.
//
// Supplying a repair function replaces fantasy's built-in one, so every
// other call falls through to the same JSON repair fantasy would have
// performed on its own. Returning a repaired call is a suggestion: fantasy
// re-validates it and discards it if it still does not pass.
func repairToolCallAlias(_ context.Context, opts fantasy.ToolCallRepairOptions) (*fantasy.ToolCallContent, error) {
	repaired := opts.OriginalToolCall

	if repaired.ToolName == readToolCallName && toolAvailable(opts.AvailableTools, tools.ViewToolName) {
		input, err := repairJSON(repaired.Input)
		if err != nil {
			return nil, err
		}
		if input, err = normalizeViewParams(input); err != nil {
			return nil, err
		}
		repaired.ToolName = tools.ViewToolName
		repaired.Input = input
		return &repaired, nil
	}

	// Everything else: fall back to fantasy's default repair so malformed
	// JSON still gets a second chance.
	input, err := repairJSON(repaired.Input)
	if err != nil || input == repaired.Input {
		return nil, err
	}
	repaired.Input = input
	return &repaired, nil
}

// toolAvailable reports whether name is among the tools enabled for this
// step. An aliased call is only rewritten when view is actually available:
// routing it to a disabled tool would fail validation anyway.
func toolAvailable(available []fantasy.AgentTool, name string) bool {
	return slices.ContainsFunc(available, func(t fantasy.AgentTool) bool {
		return t.Info().Name == name
	})
}

// repairJSON returns input as valid JSON, running it through jsonrepair when
// it is not already valid. Input that cannot be repaired yields an error so
// fantasy reports the original validation failure.
func repairJSON(input string) (string, error) {
	if json.Valid([]byte(input)) {
		return input, nil
	}
	return jsonrepair.RepairJSON(input)
}

// normalizeViewParams rewrites the argument names models habitually send
// with a "read" call into the ones view declares. Unknown parameters are
// preserved so nothing is silently dropped.
func normalizeViewParams(input string) (string, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", err
	}
	for alias, canonical := range viewParamAliases {
		value, ok := params[alias]
		if !ok {
			continue
		}
		// A canonical value the model also sent wins over the alias.
		if _, exists := params[canonical]; !exists {
			params[canonical] = value
		}
		delete(params, alias)
	}
	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
