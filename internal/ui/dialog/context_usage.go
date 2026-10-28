package dialog

import (
	"fmt"
	"math"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/workspace"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	// ContextUsageID is the identifier for the context usage dialog.
	ContextUsageID = "context_usage"

	// contextUsageBarWidth is the fixed width of the load bar, in cells.
	contextUsageBarWidth = 50

	// contextUsageDialogMaxWidth clamps the dialog width.
	contextUsageDialogMaxWidth = 62
)

// contextUsageCategory describes one row of the breakdown. The bullet
// distinguishes conversation content (◉) from the static scaffolding
// sent with every request (⛁).
type contextUsageCategory struct {
	name   string
	bullet string
	tokens int64
	style  lipgloss.Style
}

// ContextUsage is a dialog showing an estimated breakdown of how the
// session's context window is filled. Each category is rendered in its
// own color, both as a segment of the load bar and as a detail row.
type ContextUsage struct {
	com           *common.Common
	help          help.Model
	usage         workspace.ContextUsage
	contextWindow int64
	modelName     string

	keyMap struct {
		Close key.Binding
	}
}

var _ Dialog = (*ContextUsage)(nil)

// NewContextUsage creates a new context usage dialog.
func NewContextUsage(com *common.Common, usage workspace.ContextUsage, contextWindow int64, modelName string) *ContextUsage {
	c := &ContextUsage{
		com:           com,
		usage:         usage,
		contextWindow: contextWindow,
		modelName:     modelName,
	}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	c.help = h

	c.keyMap.Close = CloseKey
	return c
}

// ID implements [Dialog].
func (*ContextUsage) ID() string {
	return ContextUsageID
}

// SetUsage updates the breakdown shown by the dialog, e.g. when a
// session usage event arrives while the dialog is open.
func (c *ContextUsage) SetUsage(usage workspace.ContextUsage) {
	c.usage = usage
}

// SetContextWindow updates the context window the bar is scaled to.
func (c *ContextUsage) SetContextWindow(contextWindow int64) {
	c.contextWindow = contextWindow
}

// HandleMsg implements [Dialog].
func (c *ContextUsage) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(msg, c.keyMap.Close) {
			return ActionClose{}
		}
	}
	return nil
}

// Draw implements [Dialog].
func (c *ContextUsage) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := c.com.Styles
	width := max(0, min(contextUsageDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	rc := NewRenderContext(t, width)
	rc.Title = "Context Usage"
	rc.AddPart(c.renderSubtitle(t, innerWidth))
	rc.AddPart(c.renderBar(t, innerWidth))
	rc.AddPart(c.renderDetails(t, innerWidth))
	rc.AddPart(c.renderHint(t, innerWidth))
	rc.Help = renderDialogHelp(t, &c.help, c, innerWidth)

	view := rc.Render()
	DrawCenter(scr, area, view)
	return nil
}

// ShortHelp implements [help.KeyMap].
func (c *ContextUsage) ShortHelp() []key.Binding {
	return []key.Binding{c.keyMap.Close}
}

// FullHelp implements [help.KeyMap].
func (c *ContextUsage) FullHelp() [][]key.Binding {
	return [][]key.Binding{{c.keyMap.Close}}
}

// categories returns the breakdown rows in display order, each bound to
// the category's style and token count.
func (c *ContextUsage) categories() []contextUsageCategory {
	u := c.com.Styles.Dialog.ContextUsage
	usage := c.usage
	return []contextUsageCategory{
		{name: "User messages", bullet: "◉", tokens: usage.UserMessages, style: u.UserMessages},
		{name: "Agent responses", bullet: "◉", tokens: usage.AgentResponses, style: u.AgentResponses},
		{name: "Tool calls", bullet: "◉", tokens: usage.ToolCalls, style: u.ToolCalls},
		{name: "System prompt", bullet: "⛁", tokens: usage.SystemPrompt, style: u.SystemPrompt},
		{name: "System tools", bullet: "⛁", tokens: usage.SystemTools, style: u.SystemTools},
		{name: "Skills", bullet: "⛁", tokens: usage.Skills, style: u.Skills},
		{name: "Subagents", bullet: "⛁", tokens: usage.Subagents, style: u.Subagents},
		{name: "System messages", bullet: "◉", tokens: usage.SystemMessages, style: u.SystemMessages},
	}
}

// renderSubtitle renders the model name and context window size.
func (c *ContextUsage) renderSubtitle(t *styles.Styles, contentWidth int) string {
	if c.modelName == "" && c.contextWindow <= 0 {
		return ""
	}
	info := fmt.Sprintf("%s · %s context", c.modelName, formatContextTokens(c.contextWindow))
	return t.Dialog.SecondaryText.Width(contentWidth).Render(info)
}

// renderBar renders the segmented load bar with the total percentage.
func (c *ContextUsage) renderBar(t *styles.Styles, contentWidth int) string {
	u := t.Dialog.ContextUsage

	// The bar is scaled to the context window so the fill level matches
	// the percentage shown next to it. Without a context window it falls
	// back to the total, filling at most the whole bar.
	denominator := c.contextWindow
	if denominator <= 0 {
		denominator = c.usage.Total()
	}

	var bar strings.Builder
	filled := 0
	for _, cat := range c.categories() {
		cells := 0
		if denominator > 0 && cat.tokens > 0 {
			cells = int(math.Round(float64(cat.tokens) * float64(contextUsageBarWidth) / float64(denominator)))
		}
		cells = min(cells, contextUsageBarWidth-filled)
		if cells > 0 {
			bar.WriteString(cat.style.Render(strings.Repeat("█", cells)))
			filled += cells
		}
	}
	if track := contextUsageBarWidth - filled; track > 0 {
		bar.WriteString(u.BarTrack.Render(strings.Repeat("░", track)))
	}

	barLine := fmt.Sprintf("%s %s", bar.String(), u.PercentText.Render(formatPercent(c.usage.Total(), c.contextWindow)))
	return contentStyle(contentWidth).Render(barLine)
}

// renderDetails renders one row per category, colored to match its bar
// segment.
func (c *ContextUsage) renderDetails(t *styles.Styles, contentWidth int) string {
	u := t.Dialog.ContextUsage
	rows := make([]string, 0, len(c.categories()))
	for _, cat := range c.categories() {
		label := cat.style.Render(fmt.Sprintf("%s %s", cat.bullet, cat.name))
		value := u.DetailValue.Render(fmt.Sprintf(
			"%s tokens (%s)",
			formatContextTokens(cat.tokens),
			formatPercent(cat.tokens, c.contextWindow),
		))
		rows = append(rows, fmt.Sprintf("%s: %s", label, value))
	}
	return contentStyle(contentWidth).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// renderHint renders the estimate disclaimer.
func (c *ContextUsage) renderHint(t *styles.Styles, contentWidth int) string {
	return t.Dialog.SecondaryText.Width(contentWidth).Render(
		"Estimates only. Actual usage varies by provider.",
	)
}

// contentStyle pads dialog content lines so they line up inside the
// dialog frame.
func contentStyle(contentWidth int) lipgloss.Style {
	return lipgloss.NewStyle().Padding(0, 1).Width(contentWidth)
}

// formatContextTokens formats a token count with K/M units, always
// keeping one decimal for compactness above a thousand.
func formatContextTokens(tokens int64) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

// formatPercent formats tokens as a share of the context window.
func formatPercent(tokens, contextWindow int64) string {
	if contextWindow <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", float64(tokens)/float64(contextWindow)*100)
}
