package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parmeet20/dockcode/agent"
)

type SessionBrowserMsg struct{ SessionID string }
type SessionBrowserCloseMsg struct{}
type SessionBrowserModel struct {
	index       *agent.SessionIndex
	sessions    []agent.SessionSummary
	selected    int
	search      textinput.Model
	width       int
	height      int
	inputActive bool
}

func NewSessionBrowserModel(index *agent.SessionIndex) SessionBrowserModel {
	si := textinput.New()
	si.Placeholder = "Search sessions..."
	si.Width = 50
	si.Focus()

	m := SessionBrowserModel{
		index:       index,
		search:      si,
		inputActive: true,
	}
	m.reload()
	return m
}

func (m *SessionBrowserModel) reload() {
	all := m.index.List()
	q := strings.ToLower(m.search.Value())
	if q == "" {
		m.sessions = all
	} else {
		var filtered []agent.SessionSummary
		for _, s := range all {
			if strings.Contains(strings.ToLower(s.Title), q) {
				filtered = append(filtered, s)
			}
		}
		m.sessions = filtered
	}
	if m.selected >= len(m.sessions) {
		m.selected = len(m.sessions) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m SessionBrowserModel) Init() tea.Cmd { return nil }

func (m SessionBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.inputActive {
			switch msg.String() {
			case "ctrl+c":
				return m, func() tea.Msg { return SessionBrowserCloseMsg{} }
			case "esc", "tab":
				m.inputActive = false
				m.search.Blur()
				return m, nil
			case "up":
				if m.selected > 0 {
					m.selected--
				}
				return m, nil
			case "down":
				if m.selected < len(m.sessions)-1 {
					m.selected++
				}
				return m, nil
			case "enter":
				if len(m.sessions) > 0 {
					id := m.sessions[m.selected].ID
					return m, func() tea.Msg { return SessionBrowserMsg{SessionID: id} }
				}
				m.inputActive = false
				m.search.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				m.reload()
				return m, cmd
			}
		} else {
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				return m, func() tea.Msg { return SessionBrowserCloseMsg{} }

			case "up", "k":
				if m.selected > 0 {
					m.selected--
				}

			case "down", "j":
				if m.selected < len(m.sessions)-1 {
					m.selected++
				}

			case "enter":
				if len(m.sessions) > 0 {
					id := m.sessions[m.selected].ID
					return m, func() tea.Msg { return SessionBrowserMsg{SessionID: id} }
				}

			case "n":
				return m, func() tea.Msg { return SessionBrowserMsg{SessionID: "new"} }

			case "/", "i", "tab":
				m.inputActive = true
				m.search.Focus()
				return m, nil
			}
		}
	}
	return m, nil
}

func (m SessionBrowserModel) View() string {
	browserTitle := "Sessions"
	if HasUnicodeSupport() {
		browserTitle = "📋  Sessions"
	}

	maxRows := m.height - 16
	if maxRows < 1 {
		maxRows = 1
	}

	startIdx := 0
	if m.selected >= maxRows {
		startIdx = m.selected - maxRows + 1
	}
	endIdx := startIdx + maxRows
	if endIdx > len(m.sessions) {
		endIdx = len(m.sessions)
		startIdx = endIdx - maxRows
		if startIdx < 0 {
			startIdx = 0
		}
	}

	showingText := "0 sessions"
	if len(m.sessions) > 0 {
		showingText = fmt.Sprintf("Showing %d-%d of %d", startIdx+1, endIdx, len(m.sessions))
	}

	header := StylePrimary.Render(browserTitle) + "  " +
		StyleDim.Render("Tab/Esc=nav/search  Enter=open  N=new  Q=back") +
		"  " + StyleDim.Render("("+showingText+")")

	var searchBox string
	if m.inputActive {
		searchBox = StyleInputFocused.Width(60).Render(m.search.View())
	} else {
		searchBox = StyleInput.Width(60).Render(m.search.View())
	}

	var rows []string
	rows = append(rows, StyleBold.Render(
		fmt.Sprintf("  %-40s  %-12s  %s", "Title", "Updated", "Tokens"),
	))
	rows = append(rows, StyleDim.Render(strings.Repeat("─", 70)))

	for i := startIdx; i < endIdx; i++ {
		s := m.sessions[i]
		title := s.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		date := s.UpdatedAt
		if len(date) > 10 {
			date = date[:10]
		}
		tokens := fmt.Sprintf("%d", s.TokensIn+s.TokensOut)
		row := fmt.Sprintf("  %-40s  %-12s  %s", title, date, tokens)
		arrow := "> "
		if HasUnicodeSupport() {
			arrow = "▸ "
		}
		if i == m.selected {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				Render(arrow+row))
		} else {
			rows = append(rows, StyleBase.Render("  "+row))
		}
	}

	if len(m.sessions) == 0 {
		rows = append(rows, StyleDim.Render("  No sessions found."))
	}

	body := strings.Join(rows, "\n")
	content := header + "\n" + searchBox + "\n" + body

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorDim).
		Padding(1, 2).
		Width(m.width - 4).
		Height(m.height - 4).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
