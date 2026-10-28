package dialog

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/workspace"
)

func newTestContextUsage() *ContextUsage {
	s := styles.CharmtonePantera()
	com := &common.Common{Styles: &s}
	return NewContextUsage(com, workspace.ContextUsage{
		UserMessages:   920,
		AgentResponses: 34_600,
		ToolCalls:      48_000,
		SystemPrompt:   11_900,
		SystemTools:    11_900,
		Skills:         3_000,
		Subagents:      686,
		SystemMessages: 4_800,
	}, 1_000_000, "test-model")
}

func TestContextUsage_Bar(t *testing.T) {
	t.Parallel()

	c := newTestContextUsage()
	plain := ansi.Strip(c.renderBar(c.com.Styles, 60))

	// The bar itself spans exactly the fixed number of cells, filled
	// segments plus track, before the percentage label.
	var filled, track int
	for _, r := range plain {
		switch r {
		case '█':
			filled++
		case '░':
			track++
		}
	}
	require.Equal(t, contextUsageBarWidth, filled+track)
	require.Equal(t, 6, filled) // ~11.6% of the window.
	require.Equal(t, 44, track) // the unused remainder
	require.Contains(t, plain, "11.6%")
}

func TestContextUsage_DetailRows(t *testing.T) {
	t.Parallel()

	c := newTestContextUsage()
	details := ansi.Strip(c.renderDetails(c.com.Styles, 60))

	require.Contains(t, details, "◉ User messages: 920 tokens (0.1%)")
	require.Contains(t, details, "◉ Agent responses: 34.6K tokens (3.5%)")
	require.Contains(t, details, "◉ Tool calls: 48.0K tokens (4.8%)")
	require.Contains(t, details, "⛁ System prompt: 11.9K tokens (1.2%)")
	require.Contains(t, details, "⛁ System tools: 11.9K tokens (1.2%)")
	require.Contains(t, details, "⛁ Skills: 3.0K tokens (0.3%)")
	require.Contains(t, details, "⛁ Subagents: 686 tokens (0.1%)")
	require.Contains(t, details, "◉ System messages: 4.8K tokens (0.5%)")
}

func TestContextUsage_SetUsage(t *testing.T) {
	t.Parallel()

	c := newTestContextUsage()
	c.SetUsage(workspace.ContextUsage{UserMessages: 2_000_000})
	require.Equal(t, int64(2_000_000), c.usage.Total())
}

func TestContextUsage_BarFillsWithoutContextWindow(t *testing.T) {
	t.Parallel()

	c := newTestContextUsage()
	c.SetContextWindow(0)
	plain := ansi.Strip(c.renderBar(c.com.Styles, 60))

	// Without a context window the total becomes the denominator, so
	// the bar fills (subject to rounding) and the percentage falls
	// back to a dash.
	require.Contains(t, plain, "—")
	require.Equal(t, contextUsageBarWidth, strings.Count(plain, "█")+strings.Count(plain, "░"))
}

func TestFormatContextTokens(t *testing.T) {
	t.Parallel()

	require.Equal(t, "920", formatContextTokens(920))
	require.Equal(t, "34.6K", formatContextTokens(34_600))
	require.Equal(t, "48.0K", formatContextTokens(48_000))
	require.Equal(t, "1.0K", formatContextTokens(1_000))
	require.Equal(t, "2.0M", formatContextTokens(2_000_000))
}

func TestFormatPercent(t *testing.T) {
	t.Parallel()

	require.Equal(t, "—", formatPercent(1, 0))
	require.Equal(t, "8.3%", formatPercent(83_000, 1_000_000))
	require.Equal(t, "100.0%", formatPercent(1_000_000, 1_000_000))
}

func TestContextUsage_LineWidth(t *testing.T) {
	t.Parallel()

	c := newTestContextUsage()
	for _, line := range []string{
		c.renderSubtitle(c.com.Styles, 60),
		c.renderBar(c.com.Styles, 60),
		c.renderDetails(c.com.Styles, 60),
		c.renderHint(c.com.Styles, 60),
	} {
		require.LessOrEqual(t, lipgloss.Width(line), 60)
	}
}
