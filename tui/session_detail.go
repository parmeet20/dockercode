package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parmeet20/dockcode/agent"
)

type SessionDetailCloseMsg struct{}
type SessionDetailOpenMsg struct{ SessionID string }
type SessionDetailModel struct {
	summary agent.SessionSummary
	chatMD  string
	agentMD string
	scroll  int
	width   int
	height  int
}

func NewSessionDetailModel(
	summary agent.SessionSummary,
	chatMD string,
	agentMD string,
) SessionDetailModel {
	return SessionDetailModel{
		summary: summary,
		chatMD:  chatMD,
		agentMD: agentMD,
	}
}

func (m SessionDetailModel) Init() tea.Cmd { return nil }

func (m SessionDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, func() tea.Msg { return SessionDetailCloseMsg{} }
		case "o", "enter":
			return m, func() tea.Msg { return SessionDetailOpenMsg{SessionID: m.summary.ID} }
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		case "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m SessionDetailModel) View() string {
	halfW := m.width/2 - 4
	left := m.renderMeta(halfW)
	right := m.renderPreview(halfW)

	row := lipgloss.JoinHorizontal(lipgloss.Top,
		StyleInactiveBorder.Width(halfW).Height(m.height-6).Render(left),
		StyleActiveBorder.Width(halfW).Height(m.height-6).Render(right),
	)

	help := StyleDim.Render("O=open session  Q=back  ↑↓=scroll")
	return lipgloss.JoinVertical(lipgloss.Left,
		StyleDim.Render("◈ Session Detail"),
		row,
		help,
	)
}

func (m SessionDetailModel) renderMeta(w int) string {
	s := m.summary
	tags := strings.Join(s.Tags, ", ")
	if tags == "" {
		tags = "(none)"
	}
	lines := []string{
		StyleBold.Render("Title:  ") + s.Title,
		StyleBold.Render("ID:     ") + s.ID,
		StyleBold.Render("Updated:") + s.UpdatedAt,
		StyleBold.Render("Tokens: ") + fmt.Sprintf("in:%d out:%d", s.TokensIn, s.TokensOut),
		StyleBold.Render("Tags:   ") + tags,
		"",
		StyleDim.Render("── Agent Memory (first 20 lines) ──"),
	}
	agentLines := strings.Split(m.agentMD, "\n")
	limit := 20
	if len(agentLines) < limit {
		limit = len(agentLines)
	}
	lines = append(lines, agentLines[:limit]...)
	return strings.Join(lines, "\n")
}

func (m SessionDetailModel) renderPreview(w int) string {
	lines := strings.Split(m.chatMD, "\n")
	available := m.height - 10
	if available < 1 {
		available = 1
	}
	start := m.scroll
	if start >= len(lines) {
		start = len(lines) - 1
	}
	if start < 0 {
		start = 0
	}
	end := start + available
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}
