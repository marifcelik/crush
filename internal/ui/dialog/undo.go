package dialog

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
)

// UndoID is the identifier for the undo confirmation dialog.
const UndoID = "undo"

// Undo represents a dialog asking the user what an undo should revert:
// only the conversation, or the conversation plus the file changes made
// during the undone turn.
type Undo struct {
	com *common.Common
	// selected indexes the undo scope: 0 is conversation only, 1 reverts
	// file changes too.
	selected int
	keyMap   struct {
		LeftRight,
		EnterSpace,
		Close key.Binding
	}
}

var _ Dialog = (*Undo)(nil)

// NewUndo creates a new undo confirmation dialog.
func NewUndo(com *common.Common) *Undo {
	u := &Undo{
		com: com,
	}
	u.keyMap.LeftRight = key.NewBinding(
		key.WithKeys("left", "right", "tab"),
		key.WithHelp("←/→/tab", "switch scope"),
	)
	u.keyMap.EnterSpace = key.NewBinding(
		key.WithKeys("enter", " "),
		key.WithHelp("enter/space", "confirm"),
	)
	u.keyMap.Close = CloseKey
	return u
}

// ID implements [Model].
func (*Undo) ID() string {
	return UndoID
}

// HandleMsg implements [Model].
func (u *Undo) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, u.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, u.keyMap.LeftRight):
			u.selected = (u.selected + 1) % 2
		case key.Matches(msg, u.keyMap.EnterSpace):
			return ActionUndoChoice{RevertFiles: u.selected == 1}
		}
	}

	return nil
}

// Draw implements [Dialog].
func (u *Undo) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	const question = "Undo the last message?"
	hint := "File changes made since the message can be reverted too."

	buttonOpts := []common.ButtonOpts{
		{Text: "Conversation only", Selected: u.selected == 0, Padding: 3},
		{Text: "Conversation and files", Selected: u.selected == 1, Padding: 3},
	}
	buttons := common.ButtonGroup(u.com.Styles, buttonOpts, " ")

	content := u.com.Styles.Dialog.Quit.Content.Render(
		lipgloss.JoinVertical(
			lipgloss.Center,
			question,
			"",
			buttons,
			"",
			u.com.Styles.Dialog.Quit.Hint.Render(hint),
		),
	)

	frameStyle := u.com.Styles.Dialog.Quit.Frame
	maxWidth := area.Dx() - frameStyle.GetHorizontalBorderSize()
	if maxWidth < lipgloss.Width(content) {
		frameStyle = frameStyle.Padding(1, 0)
	}
	view := frameStyle.Render(content)
	DrawCenter(scr, area, view)
	return nil
}

// ShortHelp implements [help.KeyMap].
func (u *Undo) ShortHelp() []key.Binding {
	return []key.Binding{
		u.keyMap.LeftRight,
		u.keyMap.EnterSpace,
	}
}

// FullHelp implements [help.KeyMap].
func (u *Undo) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{u.keyMap.LeftRight, u.keyMap.EnterSpace, u.keyMap.Close},
	}
}
