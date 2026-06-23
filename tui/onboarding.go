package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parmeet20/dockcode/config"
	"github.com/parmeet20/dockcode/docker"
	"github.com/parmeet20/dockcode/llm"
)

type onboardStep int

const (
	stepWelcome onboardStep = iota
	stepBaseURL
	stepToken
	stepValidating
	stepModel
	stepDocker
	stepDone
)

type OnboardingModel struct {
	ctx         context.Context
	cfg         *config.Manager
	step        onboardStep
	width       int
	height      int
	input       textinput.Model
	maskedInput textinput.Model
	models      []string
	modelIdx    int
	errMsg      string
	statusMsg   string
	spinner     Spinner
	spinning    bool
	baseURL     string
	token       string
	model       string
}

func NewOnboardingModel(ctx context.Context, cfg *config.Manager) OnboardingModel {
	inp := textinput.New()
	inp.Placeholder = "https://api.openai.com/v1"
	inp.CharLimit = 256
	inp.Width = 60

	masked := textinput.New()
	masked.Placeholder = "sk-..."
	masked.EchoMode = textinput.EchoPassword
	masked.EchoCharacter = '●'
	masked.CharLimit = 256
	masked.Width = 60

	return OnboardingModel{
		ctx:         ctx,
		cfg:         cfg,
		step:        stepWelcome,
		input:       inp,
		maskedInput: masked,
		baseURL:     "https://api.openai.com/v1",
		spinner:     NewSpinner(),
	}
}

type OnboardDoneMsg struct {
	BaseURL string
	Token   string
	Model   string
}

type onboardValidatedMsg struct{ models []string }
type onboardValidationErrMsg struct{ err string }
type onboardDockerOKMsg struct{}
type onboardDockerErrMsg struct{ err string }

func (m OnboardingModel) Init() tea.Cmd { return nil }

func (m OnboardingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		m.errMsg = ""
		switch m.step {
		case stepWelcome:
			if msg.String() == "enter" {
				m.step = stepBaseURL
				m.input.SetValue("")
				m.input.Placeholder = "https://api.openai.com/v1"
				m.input.Focus()
			}

		case stepBaseURL:
			if msg.String() == "enter" {
				val := strings.TrimSpace(m.input.Value())
				if val == "" {
					val = "https://api.openai.com/v1"
				}
				m.baseURL = val
				m.step = stepToken
				m.maskedInput.SetValue("")
				m.maskedInput.Focus()
			} else {
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)
			}

		case stepToken:
			if msg.String() == "enter" {
				m.token = strings.TrimSpace(m.maskedInput.Value())
				if m.token == "" {
					m.errMsg = "Token cannot be empty."
					break
				}
				m.step = stepValidating
				m.spinning = true
				m.statusMsg = IconInfo + "  Validating credentials..."
				cmds = append(cmds, m.validateCmd())
			} else {
				var cmd tea.Cmd
				m.maskedInput, cmd = m.maskedInput.Update(msg)
				cmds = append(cmds, cmd)
			}

		case stepModel:
			switch msg.String() {
			case "up", "k":
				if m.modelIdx > 0 {
					m.modelIdx--
				}
			case "down", "j":
				if m.modelIdx < len(m.models)-1 {
					m.modelIdx++
				}
			case "enter":
				if len(m.models) > 0 {
					m.model = m.models[m.modelIdx]
				}
				m.step = stepDocker
				cmds = append(cmds, m.checkDockerCmd())
			}

		case stepDocker:
			if msg.String() == "r" || msg.String() == "R" {
				cmds = append(cmds, m.checkDockerCmd())
			}

		case stepDone:
		}

		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case SidebarTickMsg:
		if m.spinning {
			m.spinner.Tick()
			cmds = append(cmds, SidebarTickCmd())
		}

	case onboardValidatedMsg:
		m.spinning = false
		m.models = msg.models
		m.statusMsg = StyleSuccess.Render(IconSuccess + " Connected successfully")
		for i, mod := range m.models {
			low := strings.ToLower(mod)
			if strings.Contains(low, "gpt-4o") || strings.Contains(low, "claude") || strings.Contains(low, "llama") {
				m.modelIdx = i
				break
			}
		}
		m.step = stepModel

	case onboardValidationErrMsg:
		m.spinning = false
		m.errMsg = msg.err
		m.step = stepToken
		m.maskedInput.SetValue("")
		m.maskedInput.Focus()

	case onboardDockerOKMsg:
		m.step = stepDone
		_ = m.cfg.Update(func(c *config.AppConfig) {
			c.APIURL = m.baseURL
			c.APIToken = m.token
			c.Model = m.model
		})
		cmds = append(cmds, func() tea.Msg {
			time.Sleep(600 * time.Millisecond)
			return tea.Quit()
		})

	case onboardDockerErrMsg:
		m.statusMsg = StyleError.Render(IconErrMsg+" Docker not found: "+msg.err) +
			"\n" + StyleDim.Render("Start Docker Desktop and press R to retry.")
	}

	return m, tea.Batch(cmds...)
}

func (m OnboardingModel) View() string {
	var sb strings.Builder
	sb.WriteString(m.header())
	sb.WriteString("\n\n")

	switch m.step {
	case stepWelcome:
		sb.WriteString(m.renderWelcome())
	case stepBaseURL:
		sb.WriteString(m.renderBaseURL())
	case stepToken, stepValidating:
		sb.WriteString(m.renderToken())
	case stepModel:
		sb.WriteString(m.renderModelPicker())
	case stepDocker:
		sb.WriteString(StyleDim.Render("Checking Docker daemon..."))
	case stepDone:
		sb.WriteString(StyleSuccess.Render(IconSuccess + " All set! Starting DockCode..."))
	}

	if m.errMsg != "" {
		sb.WriteString("\n\n")
		sb.WriteString(StyleError.Render(IconErrMsg + " " + m.errMsg))
	}
	if m.statusMsg != "" {
		sb.WriteString("\n\n")
		sb.WriteString(m.statusMsg)
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorDim).
		Padding(2, 4).
		Width(70).
		Render(sb.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m OnboardingModel) header() string {
	whale := renderPixelWhale()
	logo := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("D O C K C O D E")
	sub := StyleDim.Render("AI-powered Docker management in your terminal")

	return whale + "\n" +
		lipgloss.NewStyle().Align(lipgloss.Center).Width(62).Render(logo) + "\n" +
		lipgloss.NewStyle().Align(lipgloss.Center).Width(62).Render(sub)
}

func renderPixelWhale() string {
	var sb strings.Builder
	d := lipgloss.NewStyle().Background(lipgloss.Color("#5C4033")).Render("  ")
	m := lipgloss.NewStyle().Background(lipgloss.Color("#8B5A2B")).Render("  ")
	l := lipgloss.NewStyle().Background(lipgloss.Color("#CD853F")).Render("  ")
	g := lipgloss.NewStyle().Background(lipgloss.Color("#FFD700")).Render("  ")
	b := lipgloss.NewStyle().Background(lipgloss.Color("#E3A869")).Render("  ")
	e := "  "

	grid := []string{
		"          b             ",
		"        ddddddd         ",
		"      ddddddddddd       ",
		"    ddddddddddddddd     ",
		"  ddddgdddddddddddddd   ",
		"  ddddddddddddddddmmll  ",
		"    ddddddddddddmmll    ",
		"      dddd  dddd        ",
	}

	for _, row := range grid {
		sb.WriteString("       ")
		for _, ch := range row {
			switch ch {
			case 'd':
				sb.WriteString(d)
			case 'm':
				sb.WriteString(m)
			case 'l':
				sb.WriteString(l)
			case 'g':
				sb.WriteString(g)
			case 'b':
				sb.WriteString(b)
			default:
				sb.WriteString(e)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m OnboardingModel) renderWelcome() string {
	return StyleBase.Render("Welcome! Let's get you set up in a few quick steps.") +
		"\n\n" + StyleDim.Render("[Press Enter to begin]")
}

func (m OnboardingModel) renderBaseURL() string {
	hint := StyleDim.Render("Or use http://localhost:11434/v1 for Ollama")
	return StyleBold.Render("Base URL") + "\n" + hint + "\n\n" + m.input.View()
}

func (m OnboardingModel) renderToken() string {
	hint := StyleDim.Render("Your OpenAI / Groq / etc. API key")
	busy := ""
	if m.spinning {
		busy = "\n" + m.spinner.View() + " " + m.statusMsg
	}
	return StyleBold.Render("API Token") + "\n" + hint + "\n\n" + m.maskedInput.View() + busy
}

func (m OnboardingModel) renderModelPicker() string {
	var sb strings.Builder
	sb.WriteString(StyleBold.Render("Select Model"))
	sb.WriteString("\n")
	sb.WriteString(StyleDim.Render("↑↓ to navigate, Enter to confirm"))
	sb.WriteString("\n\n")

	maxItems := 12
	start := 0
	end := len(m.models)
	if len(m.models) > maxItems {
		start = m.modelIdx - maxItems/2
		if start < 0 {
			start = 0
		}
		end = start + maxItems
		if end > len(m.models) {
			end = len(m.models)
			start = end - maxItems
		}
	}

	if start > 0 {
		sb.WriteString(StyleDim.Render(fmt.Sprintf("  ▲ ... and %d more above", start)))
		sb.WriteString("\n")
	}
	for i := start; i < end; i++ {
		mod := m.models[i]
		prefix := "  "
		if i == m.modelIdx {
			if HasUnicodeSupport() {
				prefix = StyleDim.Render("▸ ")
			} else {
				prefix = StyleDim.Render("> ")
			}
		} else {
		}
		label := mod
		if len(label) > 55 {
			label = label[:55]
		}
		if i == m.modelIdx {
			sb.WriteString(prefix)
			sb.WriteString(StyleBase.Render(label))
			sb.WriteString("\n")
		} else {
			sb.WriteString(prefix)
			sb.WriteString(StyleDim.Render(label))
			sb.WriteString("\n")
		}
	}
	if end < len(m.models) {
		sb.WriteString(StyleDim.Render(fmt.Sprintf("  ▼ ... and %d more below", len(m.models)-end)))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m OnboardingModel) validateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()

		client := llm.NewClient(m.baseURL, m.token, "")
		models, err := client.ListModels(ctx)
		if err != nil {
			return onboardValidationErrMsg{err: err.Error()}
		}
		return onboardValidatedMsg{models: models}
	}
}

func (m OnboardingModel) checkDockerCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		dc, err := docker.NewClient()
		if err != nil {
			return onboardDockerErrMsg{err: err.Error()}
		}
		defer dc.Close()
		if err := dc.Ping(ctx); err != nil {
			return onboardDockerErrMsg{err: err.Error()}
		}
		return onboardDockerOKMsg{}
	}
}
