package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPToolOutputIsBounded(t *testing.T) {
	t.Parallel()

	// A single MCP call can return megabytes (page dumps, evaluate_script
	// output). Unbounded, it lands in the session and the next request
	// exceeds the model's context window.
	huge := strings.Repeat("x", 5<<20)
	out := TruncateOutput(huge, t.TempDir())

	require.Less(t, len(out), len(huge))
	require.Contains(t, out, "lines truncated")
}
