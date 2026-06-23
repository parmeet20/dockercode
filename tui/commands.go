package tui

import (
	"strings"
)

type Command struct {
	Name string
	Desc string
}

var AllCommands = []Command{
	{"/help", "Show all available commands"},
	{"/exit", "Gracefully exit DockCode"},
	{"/clear", "Clear chat and reset session memory"},
	{"/newchat", "Start a new chat session"},
	{"/settoken", "Set a new API token"},
	{"/seturl", "Set a new API base URL"},
	{"/model", "Interactive model picker"},
	{"/models", "List available models in chat"},
	{"/config", "Show current configuration"},
	{"/theme", "Toggle dark/light theme"},
	{"/sessions", "Open session browser"},
	{"/session rename", "Rename current session"},
	{"/session delete", "Delete current session"},
	{"/session export", "Export chat to markdown file"},
	{"/session tag", "Tag current session"},
	{"/containers", "Focus containers sidebar panel"},
	{"/images", "Focus images sidebar panel"},
	{"/volumes", "Focus volumes sidebar panel"},
	{"/networks", "Focus networks sidebar panel"},
	{"/logs", "Stream container logs (usage: /logs <name>)"},
	{"/stop", "Stop a container (usage: /stop <name>)"},
	{"/rm", "Remove a container (usage: /rm <name>)"},
}

type AutocompleteState struct {
	Visible  bool
	Matches  []Command
	Selected int
}

func NewAutocompleteState() AutocompleteState {
	return AutocompleteState{}
}
func (a *AutocompleteState) Update(input string) {
	if !strings.HasPrefix(input, "/") || input == "/" {
		a.Visible = len(input) > 0 && input[0] == '/'
		if !a.Visible {
			a.Matches = nil
			return
		}
	}
	low := strings.ToLower(input)
	a.Matches = nil
	for _, cmd := range AllCommands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), low) {
			a.Matches = append(a.Matches, cmd)
		}
	}
	a.Visible = len(a.Matches) > 0
	if a.Selected >= len(a.Matches) {
		a.Selected = 0
	}
}
func (a *AutocompleteState) MoveUp() {
	if a.Selected > 0 {
		a.Selected--
	}
}
func (a *AutocompleteState) MoveDown() {
	if a.Selected < len(a.Matches)-1 {
		a.Selected++
	}
}
func (a *AutocompleteState) Current() string {
	if !a.Visible || len(a.Matches) == 0 {
		return ""
	}
	return a.Matches[a.Selected].Name
}
func (a *AutocompleteState) Hide() {
	a.Visible = false
	a.Matches = nil
	a.Selected = 0
}
func (a AutocompleteState) View(width int) string {
	if !a.Visible || len(a.Matches) == 0 {
		return ""
	}
	maxItems := 8
	if len(a.Matches) < maxItems {
		maxItems = len(a.Matches)
	}
	var sb strings.Builder
	for i := 0; i < maxItems; i++ {
		cmd := a.Matches[i]
		prefix := "  "
		name := cmd.Name
		desc := StyleDim.Render("  " + cmd.Desc)
		if i == a.Selected {
			prefix = StyleDim.Render("▸ ")
			name = StyleBase.Render(cmd.Name)
		} else {
			name = StyleDim.Render(cmd.Name)
		}
		sb.WriteString(prefix)
		sb.WriteString(name)
		sb.WriteString(desc)
		sb.WriteString("\n")
	}
	if len(a.Matches) > maxItems {
		sb.WriteString(StyleDim.Render(
			"  ... and more\n",
		))
	}
	return StyleInactiveBorder.Width(width - 2).Render(sb.String())
}
func HelpText() string {
	var sb strings.Builder
	sb.WriteString(StyleBold.Render("Available Commands"))
	sb.WriteString("\n\n")
	for _, cmd := range AllCommands {
		sb.WriteString(StyleDim.Render(cmd.Name))
		sb.WriteString(StyleDim.Render("  " + cmd.Desc))
		sb.WriteString("\n")
	}
	return sb.String()
}
func ParseSlashCommand(input string) (cmd, arg string) {
	parts := strings.SplitN(strings.TrimSpace(input), " ", 2)
	cmd = parts[0]
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}
	return
}
