package agent

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"

	"github.com/stretchr/testify/require"
)

func TestAgentToolDescriptionFor(t *testing.T) {
	t.Parallel()

	capable := Model{CatwalkCfg: catwalk.Model{
		CanReason:       true,
		ReasoningLevels: []string{"low", "high"},
		SupportsImages:  true,
	}}
	desc, err := agentToolDescriptionFor(capable)
	require.NoError(t, err)
	require.Contains(t, desc, "reasoning_effort")
	require.Contains(t, desc, "low, high")
	require.Contains(t, desc, "supports vision")

	basic := Model{}
	desc, err = agentToolDescriptionFor(basic)
	require.NoError(t, err)
	require.NotContains(t, desc, "reasoning_effort")
	require.NotContains(t, desc, "vision")
}
