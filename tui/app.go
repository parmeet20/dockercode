package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parmeet20/dockcode/agent"
	"github.com/parmeet20/dockcode/concurrency"
	"github.com/parmeet20/dockcode/config"
	"github.com/parmeet20/dockcode/docker"
	"github.com/parmeet20/dockcode/llm"
)

type viewMode int

const (
	modeChat viewMode = iota
	modeSessionBrowser
	modeSessionDetail
)

type Model struct {
	ctx            context.Context
	cancel         context.CancelFunc
	cfg            *config.Manager
	docker         *docker.Client
	llm            *llm.Client
	session        *agent.Session
	sessionIdx     *agent.SessionIndex
	agentInst      *agent.Agent
	refresher      *SidebarRefresher
	supervisor     *concurrency.Supervisor
	program        *tea.Program
	agentBusy      atomic.Bool
	tokenCount     atomic.Int64
	chat           ChatView
	sidebar        Sidebar
	statusbar      StatusBar
	input          textarea.Model
	autocomplete   AutocompleteState
	mode           viewMode
	sessionBrowser SessionBrowserModel
	sessionDetail  *SessionDetailModel
	askMsg         *agent.AskUserMsg
	askInput       string
	askOptIdx      int
	width          int
	height         int
	dockerAlive    bool
	agentRunning   bool
	spinner        Spinner
}
type ExitMsg struct{}
type SpinTickMsg struct{}

func spinTickCmd() tea.Cmd {
	return tea.Tick(TickInterval, func(_ time.Time) tea.Msg { return SpinTickMsg{} })
}
func NewModel(
	ctx context.Context,
	cancel context.CancelFunc,
	cfg *config.Manager,
	dockerClient *docker.Client,
	llmClient *llm.Client,
	sess *agent.Session,
	sessIdx *agent.SessionIndex,
	sup *concurrency.Supervisor,
) Model {
	appCfg := cfg.Get()

	inp := textarea.New()
	inp.Placeholder = "Chat with Docker…  /help for commands"
	inp.ShowLineNumbers = false
	inp.SetHeight(3)
	inp.Focus()

	statusbar := NewStatusBar(appCfg.Model)

	m := Model{
		ctx:          ctx,
		cancel:       cancel,
		cfg:          cfg,
		docker:       dockerClient,
		llm:          llmClient,
		session:      sess,
		sessionIdx:   sessIdx,
		supervisor:   sup,
		chat:         NewChatView(),
		sidebar:      NewSidebar(),
		statusbar:    statusbar,
		input:        inp,
		autocomplete: NewAutocompleteState(),
		spinner:      NewSpinner(),
	}

	m.agentInst = agent.NewAgent(
		ctx,
		llmClient,
		dockerClient,
		sess,
		sup,
		&m.agentBusy,
		&m.tokenCount,
	)

	m.chat.SetFocus(true)
	m.sidebar.SetFocus(false)

	return m
}
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
	m.agentInst.SetProgram(p)

	m.refresher = NewSidebarRefresher(m.ctx, m.docker, p, &m.agentBusy)
	m.refresher.Start()
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		spinTickCmd(),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
	case SpinTickMsg:
		m.spinner.Tick()
		m.statusbar.Update(m.tokenCount.Load(), m.dockerAlive, m.agentRunning)
		m.chat.TickThinking()
		cmds = append(cmds, spinTickCmd())
	case agent.AgentChunkMsg:
		m.chat.AppendStream(msg.Text)

	case agent.AgentDoneMsg:
		m.agentRunning = false
		m.chat.SetAgentRunning(false)
		m.chat.FlushStream()
		if msg.Err != nil {
			m.chat.AddMessage(KindError, msg.Err.Error())
		}

	case agent.ToolStartMsg:
		label := msg.Name
		if msg.Args != "" && len(msg.Args) < 80 {
			label += " " + StyleDim.Render(msg.Args)
		}
		m.chat.AddMessage(KindToolStart, label)

	case agent.ToolDoneMsg:
		if msg.Err != nil {
			m.chat.AddMessage(KindError, fmt.Sprintf("%s failed: %s", msg.Name, msg.Err))
		} else {
			preview := msg.Result
			if len(preview) > 200 {
				preview = preview[:200] + "…"
			}
			m.chat.AddMessage(KindToolDone, preview)
		}

	case agent.AskUserMsg:
		m.askMsg = &msg
		m.askInput = ""
		m.askOptIdx = 0
	case agent.SidebarRefreshMsg:
		m.sidebar.Update(msg)
		m.dockerAlive = true
	case llm.RetryMsg:
		if msg.Error {
			m.chat.AddMessage(KindError, msg.Message)
		} else {
			m.chat.AddMessage(KindInfo, msg.Message)
		}
	case SessionBrowserMsg:
		m.mode = modeChat
		if msg.SessionID == "new" {
			_ = m.session.Save()
			m.session.Stop()

			home, _ := os.UserHomeDir()
			sessionsDir := filepath.Join(home, ".dockcode", "sessions")
			sess, err := agent.NewSession(m.ctx, sessionsDir)
			if err != nil {
				m.chat.AddMessage(KindError, fmt.Sprintf("Failed to create new session: %s", err))
				break
			}
			m.session = sess
			m.agentInst = agent.NewAgent(m.ctx, m.llm, m.docker, sess, m.supervisor, &m.agentBusy, &m.tokenCount)
			m.agentInst.SetProgram(m.program)

			_ = m.sessionIdx.Upsert(agent.SessionSummary{
				ID:        sess.ID,
				Title:     "New Session",
				UpdatedAt: time.Now().Format(time.RFC3339),
			})

			m.chat = NewChatView()
			m.chat.SetSize(m.chatWidth(), m.chatHeight())
			m.chat.AddMessage(KindInfo, "Started a new chat session.")
		} else {
			_ = m.session.Save()
			m.session.Stop()

			home, _ := os.UserHomeDir()
			sessionsDir := filepath.Join(home, ".dockcode", "sessions")
			sessionPath := filepath.Join(sessionsDir, msg.SessionID)
			sess, err := agent.LoadSession(m.ctx, sessionPath)
			if err != nil {
				m.chat.AddMessage(KindError, fmt.Sprintf("Failed to load session: %s", err))
				break
			}
			m.session = sess
			m.agentInst = agent.NewAgent(m.ctx, m.llm, m.docker, sess, m.supervisor, &m.agentBusy, &m.tokenCount)
			m.agentInst.SetProgram(m.program)

			m.chat = NewChatView()
			m.chat.SetSize(m.chatWidth(), m.chatHeight())
			chatLog := sess.GetChatLog()
			for _, entry := range chatLog {
				kind := KindAssistant
				if entry.Role == "user" {
					kind = KindUser
				} else if entry.Role == "system" || entry.Role == "info" {
					kind = KindInfo
				}
				m.chat.AddMessage(kind, entry.Content)
			}
			m.chat.AddMessage(KindInfo, fmt.Sprintf("Switched to session: %s", sess.Meta.Title))
		}

	case SessionBrowserCloseMsg:
		m.mode = modeChat

	case SessionDetailCloseMsg:
		m.mode = modeSessionBrowser

	case SessionDetailOpenMsg:
		m.mode = modeChat
	case ExitMsg:
		return m, m.shutdownCmd()
	case tea.KeyMsg:
		cmd, handled := m.handleKey(msg)
		if handled {
			return m, cmd
		}
	}
	if m.mode == modeChat && m.askMsg == nil {
		if _, ok := msg.(tea.KeyMsg); ok {
			m.chat.SetFocus(true)
			m.sidebar.SetFocus(false)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
		m.autocomplete.Update(m.input.Value())
	}
	if m.mode == modeSessionBrowser {
		newBrowser, cmd := m.sessionBrowser.Update(msg)
		if sb, ok := newBrowser.(SessionBrowserModel); ok {
			m.sessionBrowser = sb
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.askMsg != nil {
		_, resCmd := m.handleAskUserKey(msg)
		return resCmd, true
	}
	if m.mode == modeSessionBrowser {
		newBrowser, cmd := m.sessionBrowser.Update(msg)
		if sb, ok := newBrowser.(SessionBrowserModel); ok {
			m.sessionBrowser = sb
		}
		return cmd, true
	}

	switch msg.String() {
	case "ctrl+c":
		return func() tea.Msg { return ExitMsg{} }, true

	case "tab":
		if m.autocomplete.Visible {
			selected := m.autocomplete.Current()
			if selected != "" {
				m.input.SetValue(selected + " ")
				m.autocomplete.Hide()
				return nil, true
			}
		}
		next := (int(m.sidebar.activePanel) + 1) % 4
		m.sidebar.SetPanel(SidebarPanel(next))
		m.sidebar.SetFocus(true)
		m.chat.SetFocus(false)
		return nil, true

	case "alt+1", "f1":
		m.sidebar.SetPanel(PanelContainers)
		m.sidebar.SetFocus(true)
		m.chat.SetFocus(false)
		return nil, true
	case "alt+2", "f2":
		m.sidebar.SetPanel(PanelImages)
		m.sidebar.SetFocus(true)
		m.chat.SetFocus(false)
		return nil, true
	case "alt+3", "f3":
		m.sidebar.SetPanel(PanelVolumes)
		m.sidebar.SetFocus(true)
		m.chat.SetFocus(false)
		return nil, true
	case "alt+4", "f4":
		m.sidebar.SetPanel(PanelNetworks)
		m.sidebar.SetFocus(true)
		m.chat.SetFocus(false)
		return nil, true

	case "up":
		if m.autocomplete.Visible {
			m.autocomplete.MoveUp()
			return nil, true
		}
		m.chat.ScrollUp()
		m.chat.SetFocus(true)
		m.sidebar.SetFocus(false)
		return nil, true
	case "down":
		if m.autocomplete.Visible {
			m.autocomplete.MoveDown()
			return nil, true
		}
		m.chat.ScrollDown()
		m.chat.SetFocus(true)
		m.sidebar.SetFocus(false)
		return nil, true

	case "esc":
		if m.autocomplete.Visible {
			m.autocomplete.Hide()
			return nil, true
		}
	case "ctrl+p":
		if m.autocomplete.Visible {
			m.autocomplete.MoveUp()
			return nil, true
		}
	case "ctrl+n":
		if m.autocomplete.Visible {
			m.autocomplete.MoveDown()
			return nil, true
		}

	case "enter":
		if m.autocomplete.Visible {
			selected := m.autocomplete.Current()
			if selected != "" && strings.TrimSpace(m.input.Value()) != selected {
				m.input.SetValue(selected + " ")
				m.autocomplete.Hide()
				return nil, true
			}
		}
		m.chat.SetFocus(true)
		m.sidebar.SetFocus(false)
		_, resCmd := m.submitInput()
		return resCmd, true
	}

	return nil, false
}

func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	raw := strings.TrimSpace(m.input.Value())
	if raw == "" {
		return m, nil
	}
	m.input.SetValue("")
	m.autocomplete.Hide()
	if strings.HasPrefix(raw, "/") {
		return m.handleSlashCommand(raw)
	}
	if m.agentRunning {
		m.chat.AddMessage(KindInfo, "Agent is busy, please wait…")
		return m, nil
	}

	m.agentRunning = true
	m.chat.SetAgentRunning(true)
	m.chat.AddMessage(KindUser, raw)

	return m, func() tea.Msg {
		m.agentInst.Run(raw)
		return nil
	}
}

func (m *Model) handleSlashCommand(raw string) (tea.Model, tea.Cmd) {
	cmd, arg := ParseSlashCommand(raw)
	switch cmd {
	case "/help":
		m.chat.AddMessage(KindInfo, HelpText())

	case "/exit":
		return m, func() tea.Msg { return ExitMsg{} }

	case "/clear":
		m.chat = NewChatView()
		m.chat.SetSize(m.chatWidth(), m.chatHeight())
		m.chat.AddMessage(KindInfo, "Chat cleared.")

	case "/theme":
		ToggleTheme()
		m.chat.AddMessage(KindInfo, "Theme toggled.")

	case "/newchat":
		_ = m.session.Save()
		m.session.Stop()

		home, _ := os.UserHomeDir()
		sessionsDir := filepath.Join(home, ".dockcode", "sessions")
		sess, err := agent.NewSession(m.ctx, sessionsDir)
		if err != nil {
			m.chat.AddMessage(KindError, fmt.Sprintf("Failed to create new session: %s", err))
			return m, nil
		}
		m.session = sess
		m.agentInst = agent.NewAgent(
			m.ctx,
			m.llm,
			m.docker,
			sess,
			m.supervisor,
			&m.agentBusy,
			&m.tokenCount,
		)
		m.agentInst.SetProgram(m.program)

		_ = m.sessionIdx.Upsert(agent.SessionSummary{
			ID:        sess.ID,
			Title:     "New Session",
			UpdatedAt: time.Now().Format(time.RFC3339),
		})

		m.chat = NewChatView()
		m.chat.SetSize(m.chatWidth(), m.chatHeight())
		m.chat.AddMessage(KindInfo, "Started a new chat session.")

	case "/sessions":
		browser := NewSessionBrowserModel(m.sessionIdx)
		browser.width = m.width
		browser.height = m.height
		m.sessionBrowser = browser
		m.mode = modeSessionBrowser

	case "/containers":
		m.sidebar.SetPanel(PanelContainers)
	case "/images":
		m.sidebar.SetPanel(PanelImages)
	case "/volumes":
		m.sidebar.SetPanel(PanelVolumes)
	case "/networks":
		m.sidebar.SetPanel(PanelNetworks)

	case "/logs":
		if arg == "" {
			m.chat.AddMessage(KindError, "Usage: /logs <container-name>")
		} else {
			m.chat.AddMessage(KindUser, fmt.Sprintf("/logs %s", arg))
			m.chat.AddMessage(KindToolStart, fmt.Sprintf("docker_logs %s", arg))
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
				defer cancel()
				logs, err := m.docker.GetContainerLogs(ctx, arg, 100)
				if err != nil {
					return agent.ToolDoneMsg{Name: "docker_logs", Err: err}
				}
				return agent.ToolDoneMsg{Name: "docker_logs", Result: logs}
			}
		}

	case "/stop":
		if arg == "" {
			m.chat.AddMessage(KindError, "Usage: /stop <container-name>")
		} else {
			m.chat.AddMessage(KindUser, fmt.Sprintf("/stop %s", arg))
			m.chat.AddMessage(KindToolStart, fmt.Sprintf("docker_stop %s", arg))
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
				defer cancel()
				err := m.docker.StopContainer(ctx, arg)
				if err != nil {
					return agent.ToolDoneMsg{Name: "docker_stop", Err: err}
				}
				return agent.ToolDoneMsg{Name: "docker_stop", Result: fmt.Sprintf("Stopped container: %s", arg)}
			}
		}

	case "/rm":
		if arg == "" {
			m.chat.AddMessage(KindError, "Usage: /rm <container-name>")
		} else {
			m.chat.AddMessage(KindUser, fmt.Sprintf("/rm %s", arg))
			m.chat.AddMessage(KindToolStart, fmt.Sprintf("docker_rm %s", arg))
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
				defer cancel()
				err := m.docker.RemoveContainer(ctx, arg, true)
				if err != nil {
					return agent.ToolDoneMsg{Name: "docker_rm", Err: err}
				}
				return agent.ToolDoneMsg{Name: "docker_rm", Result: fmt.Sprintf("Removed container: %s", arg)}
			}
		}

	case "/config":
		cfg := m.cfg.Get()
		m.chat.AddMessage(KindInfo, fmt.Sprintf(
			"Base URL: %s\nModel: %s\nTheme: %s",
			cfg.APIURL, cfg.Model, cfg.Theme,
		))

	case "/settoken":
		if arg == "" {
			m.chat.AddMessage(KindError, "Usage: /settoken <api-token>")
		} else {
			_ = m.cfg.Update(func(c *config.AppConfig) {
				c.APIToken = arg
			})
			m.llm.UpdateConfig(m.cfg.Get().APIURL, arg, m.cfg.Get().Model)
			m.chat.AddMessage(KindInfo, "API token updated and saved.")
		}

	case "/seturl":
		if arg == "" {
			m.chat.AddMessage(KindError, "Usage: /seturl <base-url>")
		} else {
			_ = m.cfg.Update(func(c *config.AppConfig) {
				c.APIURL = arg
			})
			m.llm.UpdateConfig(arg, m.cfg.Get().APIToken, m.cfg.Get().Model)
			m.chat.AddMessage(KindInfo, fmt.Sprintf("API base URL updated to %s and saved.", arg))
		}

	case "/model":
		if arg == "" {
			m.chat.AddMessage(KindInfo, fmt.Sprintf("Current model: %s\nTo change it, use: /model <model-name>", m.cfg.Get().Model))
		} else {
			_ = m.cfg.Update(func(c *config.AppConfig) {
				c.Model = arg
			})
			m.llm.UpdateConfig(m.cfg.Get().APIURL, m.cfg.Get().APIToken, arg)
			m.statusbar.model = arg
			m.chat.AddMessage(KindInfo, fmt.Sprintf("Model updated to %s and saved.", arg))
		}

	case "/models":
		m.chat.AddMessage(KindUser, "/models")
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			defer cancel()
			models, err := m.llm.ListModels(ctx)
			if err != nil {
				return agent.AgentDoneMsg{Err: err}
			}
			return agent.AgentChunkMsg{Text: "Available models:\n• " + strings.Join(models, "\n• ")}
		}

	case "/session":
		parts := strings.SplitN(arg, " ", 2)
		subCmd := parts[0]
		subArg := ""
		if len(parts) > 1 {
			subArg = parts[1]
		}
		switch subCmd {
		case "rename":
			if subArg == "" {
				m.chat.AddMessage(KindError, "Usage: /session rename <new-title>")
			} else {
				m.session.SetTitle(subArg)
				_ = m.session.Save()
				meta := m.session.GetMeta()
				_ = m.sessionIdx.Upsert(agent.SessionSummary{
					ID:        meta.ID,
					Title:     meta.Title,
					UpdatedAt: time.Now().Format(time.RFC3339),
					TokensIn:  meta.TokensIn,
					TokensOut: meta.TokensOut,
					Tags:      meta.Tags,
				})
				m.chat.AddMessage(KindInfo, fmt.Sprintf("Session renamed to: %s", subArg))
			}

		case "delete":
			_ = m.sessionIdx.Delete(m.session.ID)
			m.session.Stop()
			_ = os.RemoveAll(m.session.Dir)

			home, _ := os.UserHomeDir()
			sessionsDir := filepath.Join(home, ".dockcode", "sessions")
			sess, err := agent.NewSession(m.ctx, sessionsDir)
			if err != nil {
				m.chat.AddMessage(KindError, fmt.Sprintf("Failed to create new session: %s", err))
				return m, nil
			}
			m.session = sess
			m.agentInst = agent.NewAgent(
				m.ctx,
				m.llm,
				m.docker,
				sess,
				m.supervisor,
				&m.agentBusy,
				&m.tokenCount,
			)
			m.agentInst.SetProgram(m.program)

			_ = m.sessionIdx.Upsert(agent.SessionSummary{
				ID:        sess.ID,
				Title:     "New Session",
				UpdatedAt: time.Now().Format(time.RFC3339),
			})

			m.chat = NewChatView()
			m.chat.SetSize(m.chatWidth(), m.chatHeight())
			m.chat.AddMessage(KindInfo, "Deleted current session and started a new one.")

		case "export":
			log := m.session.GetChatLog()
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("# DockCode Session Export - %s\n", m.session.GetMeta().Title))
			sb.WriteString(fmt.Sprintf("Session ID: `%s`  \nDate: %s\n\n", m.session.ID, time.Now().Format(time.RFC822)))
			for _, e := range log {
				sb.WriteString(fmt.Sprintf("### %s (%s)\n%s\n\n", strings.ToUpper(e.Role), e.Timestamp.Format("15:04:05"), e.Content))
				sb.WriteString("---\n\n")
			}
			exportFile := fmt.Sprintf("dockcode-export-%s.md", m.session.ID)
			err := os.WriteFile(exportFile, []byte(sb.String()), 0644)
			if err != nil {
				m.chat.AddMessage(KindError, fmt.Sprintf("Failed to export session: %s", err))
			} else {
				m.chat.AddMessage(KindInfo, fmt.Sprintf("Session exported successfully to: %s", exportFile))
			}

		case "tag":
			if subArg == "" {
				m.chat.AddMessage(KindError, "Usage: /session tag <tag-name>")
			} else {
				meta := m.session.GetMeta()
				meta.Tags = append(meta.Tags, subArg)
				m.session.Meta.Tags = meta.Tags
				_ = m.session.Save()
				_ = m.sessionIdx.Upsert(agent.SessionSummary{
					ID:        meta.ID,
					Title:     meta.Title,
					UpdatedAt: time.Now().Format(time.RFC3339),
					TokensIn:  meta.TokensIn,
					TokensOut: meta.TokensOut,
					Tags:      meta.Tags,
				})
				m.chat.AddMessage(KindInfo, fmt.Sprintf("Added tag: %s", subArg))
			}

		default:
			m.chat.AddMessage(KindInfo, "Unknown session sub-command. Try: rename, delete, export, tag")
		}

	default:
		m.chat.AddMessage(KindError, fmt.Sprintf("Unknown command: %s  (try /help)", cmd))
	}

	return m, nil
}

func (m *Model) handleAskUserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ask := m.askMsg
	switch msg.String() {
	case "enter":
		var answer map[string]string
		if len(ask.Options) > 0 {
			answer = map[string]string{"answer": ask.Options[m.askOptIdx]}
		} else if len(ask.Fields) > 0 {
			answer = map[string]string{"value": m.askInput}
		} else {
			answer = map[string]string{"answer": m.askInput}
		}
		m.agentInst.SubmitAskUserReply(agent.AskUserReply{Answer: answer})
		m.chat.AddMessage(KindInfo, fmt.Sprintf("You answered: %v", answer))
		m.askMsg = nil

	case "up", "k":
		if m.askOptIdx > 0 {
			m.askOptIdx--
		}
	case "down", "j":
		if m.askOptIdx < len(ask.Options)-1 {
			m.askOptIdx++
		}

	case "ctrl+c":
		return m, func() tea.Msg { return ExitMsg{} }

	case "backspace":
		if len(m.askInput) > 0 {
			m.askInput = m.askInput[:len(m.askInput)-1]
		}

	default:
		if len(msg.String()) == 1 {
			m.askInput += msg.String()
		}
	}
	return m, nil
}
func (m *Model) shutdownCmd() tea.Cmd {
	return func() tea.Msg {
		m.agentInst.Stop()
		if m.refresher != nil {
			m.refresher.Stop()
		}
		_ = m.session.Save()
		m.session.Stop()
		_ = m.docker.Close()
		return tea.Quit()
	}
}

func (m *Model) View() string {
	if m.width == 0 {
		return "Starting DockCode…"
	}

	switch m.mode {
	case modeSessionBrowser:
		return m.sessionBrowser.View()
	case modeSessionDetail:
		if m.sessionDetail != nil {
			return m.sessionDetail.View()
		}
	}
	statusLine := m.statusbar.View()
	chatArea := m.chat.View()
	sidebarArea := m.sidebar.View()
	inputArea := m.renderInput()
	keyhelp := m.renderKeyHelp()
	if m.askMsg != nil {
		chatArea = m.renderAskUserOverlay()
	}
	acDropdown := ""
	if m.autocomplete.Visible {
		acDropdown = m.autocomplete.View(m.chatWidth()) + "\n"
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top, chatArea, sidebarArea)

	return lipgloss.JoinVertical(lipgloss.Left,
		statusLine,
		top,
		acDropdown+inputArea,
		keyhelp,
	)
}

func (m *Model) renderInput() string {
	style := StyleInput
	if m.input.Focused() {
		style = StyleInputFocused
	}
	return style.Width(m.width - 2).Render(m.input.View())
}

func (m *Model) renderKeyHelp() string {
	parts := []string{
		StyleDim.Render("Tab") + " panels",
		StyleDim.Render("↑↓") + " scroll",
		StyleDim.Render("/") + "commands",
		StyleDim.Render("Ctrl+C") + " exit",
	}
	return StyleDim.Render("  " + strings.Join(parts, "  ·  "))
}

func (m *Model) renderAskUserOverlay() string {
	ask := m.askMsg
	var sb strings.Builder
	sb.WriteString(StyleToolPrefix.Render("⚙ Agent asks:"))
	sb.WriteString("\n\n")
	sb.WriteString(StyleBase.Render(ask.Question))
	sb.WriteString("\n\n")

	if len(ask.Options) > 0 {
		for i, opt := range ask.Options {
			if i == m.askOptIdx {
				sb.WriteString(StyleDim.Render("▸ " + opt))
				sb.WriteString("\n")
			} else {
				sb.WriteString(StyleDim.Render("  " + opt))
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(StyleDim.Render("↑↓ to select, Enter to confirm"))
	} else {
		sb.WriteString(StyleDim.Render("Type your answer: "))
		sb.WriteString(m.askInput)
		sb.WriteString(StyleDim.Render("█"))
		sb.WriteString("\n")
		sb.WriteString(StyleDim.Render("Enter to confirm"))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorTool).
		Padding(1, 2).
		Width(m.chatWidth() - 2).
		Height(m.chatHeight() - 2).
		Render(sb.String())
}

func (m *Model) relayout() {
	sidebarW := m.width * 30 / 100
	chatW := m.width - sidebarW

	inputH := 5
	statusH := 1
	keyHelpH := 1
	topH := m.height - inputH - statusH - keyHelpH - 2

	m.chat.SetSize(chatW, topH)
	m.sidebar.SetSize(sidebarW, topH)
	m.statusbar.SetWidth(m.width)
	m.input.SetWidth(m.width - 4)
}

func (m *Model) chatWidth() int {
	return m.width * 70 / 100
}

func (m *Model) chatHeight() int {
	return m.height - 8
}
func SessionsBaseDir() string {
	return filepath.Join("~", ".dockcode", "sessions")
}
