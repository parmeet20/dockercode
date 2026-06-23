package tui

import (
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func HasUnicodeSupport() bool {
	if runtime.GOOS != "windows" {
		return true
	}
	if os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != "" {
		return true
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "xterm") || strings.Contains(term, "256color") || term == "cygwin" {
		return true
	}
	return false
}

var (
	ColorPrimary = lipgloss.AdaptiveColor{Light: "#7A431D", Dark: "#E3A869"}
	ColorText    = lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#E8E8E8"}
	ColorDim     = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#A0A0A0"}
	ColorBg      = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0D0D0D"}
	ColorPanel   = lipgloss.AdaptiveColor{Light: "#F0F0F0", Dark: "#1A1A1A"}
	ColorInput   = lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}
	ColorSuccess = lipgloss.AdaptiveColor{Light: "#007744", Dark: "#00FF88"}
	ColorError   = lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF4444"}
	ColorWarning = lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#FFD700"}
	ColorTool    = lipgloss.AdaptiveColor{Light: "#CC5500", Dark: "#FF8C00"}
	ColorBorder  = lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#333333"}
)

var (
	AppLogo     = "DockCode"
	IconPending = "◦"
	IconSuccess = "✓"
	IconError   = "✗"
	IconUser    = "▸ You"
	IconAgent   = "◈ Docker"
	IconInfo    = "◆ Info"
	IconErrMsg  = "✖ Error"
	IconTool    = "⚙ Tool"

	SpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
)

func init() {
	if !HasUnicodeSupport() {
		AppLogo = "DockCode"
		IconPending = "-"
		IconSuccess = "[OK]"
		IconError = "[ERR]"
		IconUser = "> You"
		IconAgent = "* Docker"
		IconInfo = "i Info"
		IconErrMsg = "! Error"
		IconTool = "# Tool"
		SpinnerFrames = []string{"-", "\\", "|", "/"}
	}
}

var (
	StyleBase = lipgloss.NewStyle().
			Foreground(ColorText)

	StyleDim = lipgloss.NewStyle().
			Foreground(ColorDim)

	StylePrimary = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StyleSuccess = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	StyleError = lipgloss.NewStyle().
			Foreground(ColorError)

	StyleWarning = lipgloss.NewStyle().
			Foreground(ColorWarning)

	StyleTool = lipgloss.NewStyle().
			Foreground(ColorTool)

	StyleBold = lipgloss.NewStyle().
			Bold(true)
	StyleActiveBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary)

	StyleInactiveBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder)

	StyleActiveTab = lipgloss.NewStyle().
				Background(ColorPrimary).
				Foreground(ColorBg).
				Bold(true).
				Padding(0, 1)

	StyleInactiveTab = lipgloss.NewStyle().
				Foreground(ColorDim).
				Padding(0, 1)

	StyleStatusBar = lipgloss.NewStyle().
			Background(ColorPanel).
			Foreground(ColorText).
			Padding(0, 1)
	StyleInput = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	StyleInputFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorDim).
				Padding(0, 1)
	StyleUserPrefix  = lipgloss.NewStyle().Foreground(ColorDim).Bold(true)
	StyleAgentPrefix = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	StyleInfoPrefix  = lipgloss.NewStyle().Foreground(ColorDim)
	StyleErrPrefix   = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	StyleToolPrefix  = lipgloss.NewStyle().Foreground(ColorTool).Bold(true)
)
var IsDarkMode = true

func ToggleTheme() {
	IsDarkMode = !IsDarkMode
}
