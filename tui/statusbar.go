package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type StatusBar struct {
	width       int
	model       string
	tokenCount  int64
	tokenLimit  int64
	dockerAlive bool
	agentBusy   bool
	spinner     Spinner
}

func NewStatusBar(model string) StatusBar {
	return StatusBar{
		model:      model,
		tokenLimit: 100_000,
		spinner:    NewSpinner(),
	}
}
func (s *StatusBar) SetWidth(w int) { s.width = w }
func (s *StatusBar) Update(tokenCount int64, dockerAlive, agentBusy bool) {
	s.tokenCount = tokenCount
	s.dockerAlive = dockerAlive
	s.agentBusy = agentBusy
	if agentBusy {
		s.spinner.Tick()
	}
}
func (s StatusBar) View() string {
	left := StyleDim.Render(AppLogo) +
		StyleDim.Render("  |  ") +
		StyleBase.Render(IconInfo+" "+s.model)
	middle := s.tokenBar()
	dockerDot := StyleError.Render("● offline")
	if s.dockerAlive {
		dockerDot = StyleSuccess.Render("● docker")
	}
	right := dockerDot
	if s.agentBusy {
		right = s.spinner.View() + "  " + right
	}
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	midW := lipgloss.Width(middle)
	space := s.width - leftW - rightW - midW - 4
	if space < 1 {
		space = 1
	}
	bar := left + strings.Repeat(" ", space/2) + middle + strings.Repeat(" ", space-space/2) + right

	return StyleStatusBar.Width(s.width).Render(bar)
}

func (s StatusBar) tokenBar() string {
	if s.tokenLimit == 0 {
		return ""
	}
	ratio := float64(s.tokenCount) / float64(s.tokenLimit)
	if ratio > 1 {
		ratio = 1
	}
	barWidth := 10
	filled := int(ratio * float64(barWidth))

	var bar strings.Builder
	bar.WriteString("tokens ")

	var barColor lipgloss.AdaptiveColor
	switch {
	case ratio > 0.85:
		barColor = ColorError
	case ratio > 0.60:
		barColor = ColorWarning
	default:
		barColor = ColorSuccess
	}

	filledStyle := lipgloss.NewStyle().Foreground(barColor)
	emptyStyle := lipgloss.NewStyle().Foreground(ColorBorder)

	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar.WriteString(filledStyle.Render("█"))
		} else {
			bar.WriteString(emptyStyle.Render("░"))
		}
	}
	bar.WriteString(fmt.Sprintf(" %s / %s",
		formatTokens(s.tokenCount),
		formatTokens(s.tokenLimit),
	))
	return bar.String()
}

func formatTokens(n int64) string {
	if n >= 1000 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%d", n)
}
