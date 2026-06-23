package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var thinkingSpinner = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
var thinkingPhrases = []string{
	"Thinking",
	"Reasoning",
	"Analyzing",
	"Processing",
	"Evaluating",
	"Planning",
}

func init() {
	if !HasUnicodeSupport() {
		thinkingSpinner = []string{"-", "\\", "|", "/"}
	}
}

type ChatMessageKind int

const (
	KindUser ChatMessageKind = iota
	KindAssistant
	KindToolStart
	KindToolDone
	KindInfo
	KindError
)

type ChatMessage struct {
	Kind    ChatMessageKind
	Content string
}
type ChatView struct {
	messages     []ChatMessage
	viewport     viewport.Model
	streaming    string
	agentRunning bool
	spinFrame    int
	width        int
	height       int
	focused      bool
}

func NewChatView() ChatView {
	vp := viewport.New(0, 0)
	vp.SetContent("")
	return ChatView{viewport: vp}
}
func (c *ChatView) SetSize(w, h int) {
	c.width = w
	c.height = h
	c.viewport.Width = w - 2
	c.viewport.Height = h - 2
	c.render()
}
func (c *ChatView) SetFocus(f bool) { c.focused = f }
func (c *ChatView) SetAgentRunning(running bool) {
	c.agentRunning = running
	if !running {
		c.spinFrame = 0
	}
	c.render()
	if running {
		c.viewport.GotoBottom()
	}
}
func (c *ChatView) TickThinking() {
	if c.agentRunning && c.streaming == "" {
		c.spinFrame++
		c.render()
		c.viewport.GotoBottom()
	}
}
func (c *ChatView) AddMessage(kind ChatMessageKind, content string) {
	c.messages = append(c.messages, ChatMessage{Kind: kind, Content: content})
	c.streaming = ""
	c.render()
	c.viewport.GotoBottom()
}
func (c *ChatView) AppendStream(text string) {
	c.streaming += text
	c.render()
	c.viewport.GotoBottom()
}
func (c *ChatView) FlushStream() {
	if c.streaming != "" {
		c.messages = append(c.messages, ChatMessage{Kind: KindAssistant, Content: c.streaming})
		c.streaming = ""
		c.render()
		c.viewport.GotoBottom()
	}
}
func (c *ChatView) ScrollUp()   { c.viewport.LineUp(3) }
func (c *ChatView) ScrollDown() { c.viewport.LineDown(3) }
func (c *ChatView) render() {
	var sb strings.Builder
	for _, m := range c.messages {
		sb.WriteString(c.renderMessage(m))
		sb.WriteString("\n")
	}
	if c.agentRunning && c.streaming == "" {
		sb.WriteString(c.renderThinkingMsg())
	} else if c.streaming != "" {
		sb.WriteString(c.renderStreamingMsg())
	}
	c.viewport.SetContent(sb.String())
}

func (c ChatView) renderMessage(m ChatMessage) string {
	w := c.width - 4
	if w < 20 {
		w = 20
	}
	switch m.Kind {
	case KindUser:
		prefix := StyleUserPrefix.Render(IconUser)
		body := wrapText(m.Content, w)
		return prefix + "\n" + lipgloss.NewStyle().
			Foreground(ColorText).
			PaddingLeft(2).
			Width(w).
			Render(body) + "\n"

	case KindAssistant:
		prefix := StyleAgentPrefix.Render(IconAgent)
		body := renderMarkdownText(m.Content, w)
		return prefix + "\n" + lipgloss.NewStyle().
			PaddingLeft(2).
			Width(w).
			Render(body) + "\n"

	case KindToolStart:
		return StyleToolPrefix.Render(fmt.Sprintf("%s  %s", IconTool, m.Content)) + "\n"

	case KindToolDone:
		preview := m.Content
		ellipsis := "…"
		if !HasUnicodeSupport() {
			ellipsis = "..."
		}
		if len(preview) > 120 {
			preview = preview[:120] + ellipsis
		}
		prefix := "   └─ "
		if !HasUnicodeSupport() {
			prefix = "   +- "
		}
		return StyleDim.Render(prefix+preview) + "\n"

	case KindInfo:
		body := renderMarkdownText(m.Content, w)
		return StyleInfoPrefix.Render(IconInfo) + "  " + body + "\n"

	case KindError:
		return StyleErrPrefix.Render(IconErrMsg+"  "+m.Content) + "\n"
	}
	return ""
}
func (c ChatView) renderStreamingMsg() string {
	w := c.width - 4
	if w < 20 {
		w = 20
	}
	prefix := StyleAgentPrefix.Render(IconAgent)
	body := renderMarkdownText(c.streaming, w)
	cursor := StyleDim.Render("█")
	return prefix + "\n" + lipgloss.NewStyle().
		PaddingLeft(2).
		Width(w).
		Render(body+cursor) + "\n"
}
func (c ChatView) renderThinkingMsg() string {
	spinIdx := c.spinFrame % len(thinkingSpinner)
	phraseIdx := (c.spinFrame / (len(thinkingSpinner) * 2)) % len(thinkingPhrases)

	prefix := StyleAgentPrefix.Render(IconAgent)

	spin := lipgloss.NewStyle().Foreground(ColorDim).Bold(true).
		Render(thinkingSpinner[spinIdx])
	phrase := lipgloss.NewStyle().Foreground(ColorDim).
		Render(thinkingPhrases[phraseIdx])
	dotCount := (c.spinFrame/len(thinkingSpinner))%3 + 1
	dotChar := "·"
	if !HasUnicodeSupport() {
		dotChar = "."
	}
	dots := lipgloss.NewStyle().Foreground(ColorDim).Render(strings.Repeat(dotChar, dotCount))
	barLen := 14
	barPos := c.spinFrame % barLen
	var bar strings.Builder
	if HasUnicodeSupport() {
		for i := 0; i < barLen; i++ {
			dist := i - barPos
			if dist < 0 {
				dist = -dist
			}
			switch dist {
			case 0:
				bar.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Bold(true).Render("▰"))
			case 1:
				bar.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render("▰"))
			case 2:
				bar.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render("▱"))
			default:
				bar.WriteString(lipgloss.NewStyle().Foreground(ColorBorder).Render("▱"))
			}
		}
	} else {
		bar.WriteString("[")
		for i := 0; i < barLen; i++ {
			if i == barPos {
				bar.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Bold(true).Render("="))
			} else {
				bar.WriteString(lipgloss.NewStyle().Foreground(ColorBorder).Render("-"))
			}
		}
		bar.WriteString("]")
	}

	body := spin + "  " + phrase + dots + "   " + bar.String()
	return prefix + "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(body) + "\n"
}
func (c ChatView) View() string {
	style := StyleInactiveBorder
	if c.focused {
		style = StyleActiveBorder
	}
	return style.Width(c.width - 2).Height(c.height - 2).Render(c.viewport.View())
}
func renderMarkdownText(text string, width int) string {
	lines := strings.Split(text, "\n")
	var out []string
	inCodeBlock := false
	var codeLines []string
	codeLang := ""

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				out = append(out, renderCodeBlock(codeLines, codeLang, width))
				codeLines = nil
				codeLang = ""
				inCodeBlock = false
			} else {
				inCodeBlock = true
				codeLang = strings.TrimPrefix(line, "```")
			}
			continue
		}
		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}
		if strings.HasPrefix(line, "### ") {
			content := processInline(strings.TrimPrefix(line, "### "))
			out = append(out, lipgloss.NewStyle().Bold(true).Foreground(ColorDim).
				Render("  › "+content))
			continue
		}
		if strings.HasPrefix(line, "## ") {
			content := processInline(strings.TrimPrefix(line, "## "))
			out = append(out, lipgloss.NewStyle().Bold(true).Foreground(ColorDim).Underline(true).
				Render(" ▸ "+content))
			continue
		}
		if strings.HasPrefix(line, "# ") {
			content := processInline(strings.TrimPrefix(line, "# "))
			out = append(out, lipgloss.NewStyle().Bold(true).Foreground(ColorDim).Underline(true).
				Render("▌ "+content))
			continue
		}
		if line == "---" || line == "***" || line == "___" {
			ruleW := width
			if ruleW < 1 {
				ruleW = 40
			}
			out = append(out, StyleDim.Render(strings.Repeat("─", ruleW)))
			continue
		}
		if strings.HasPrefix(line, "> ") {
			content := processInline(strings.TrimPrefix(line, "> "))
			out = append(out, lipgloss.NewStyle().Foreground(ColorDim).Italic(true).
				Render("  │ "+content))
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			content := processInline(line[2:])
			out = append(out, StyleDim.Render("  •")+" "+content)
			continue
		}
		if len(line) >= 3 && line[0] >= '1' && line[0] <= '9' && line[1] == '.' && line[2] == ' ' {
			content := processInline(line[3:])
			out = append(out, StyleDim.Render("  "+string(line[0])+".")+" "+content)
			continue
		}
		out = append(out, processInline(line))
	}
	if inCodeBlock && len(codeLines) > 0 {
		out = append(out, renderCodeBlock(codeLines, codeLang, width))
	}

	return strings.Join(out, "\n")
}
func processInline(text string) string {
	var sb strings.Builder
	i := 0
	for i < len(text) {
		if i+3 < len(text) && text[i] == '*' && text[i+1] == '*' {
			end := strings.Index(text[i+2:], "**")
			if end >= 0 {
				sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorText).
					Render(text[i+2 : i+2+end]))
				i = i + 4 + end
				continue
			}
		}
		if text[i] == '*' && (i == 0 || text[i-1] != '*') {
			end := strings.Index(text[i+1:], "*")
			if end >= 0 {
				next := i + 1 + end + 1
				if next >= len(text) || text[next] != '*' {
					sb.WriteString(lipgloss.NewStyle().Italic(true).Foreground(ColorDim).
						Render(text[i+1 : i+1+end]))
					i = next
					continue
				}
			}
		}
		if text[i] == '`' {
			end := strings.Index(text[i+1:], "`")
			if end >= 0 {
				content := text[i+1 : i+1+end]
				sb.WriteString(lipgloss.NewStyle().Foreground(ColorWarning).
					Render("`" + content + "`"))
				i = i + 2 + end
				continue
			}
		}
		sb.WriteByte(text[i])
		i++
	}
	return sb.String()
}
func renderCodeBlock(lines []string, lang string, width int) string {
	w := width - 4
	if w < 10 {
		w = 10
	}
	var sb strings.Builder
	langLabel := ""
	if lang != "" {
		langLabel = " " + lang + " "
	}
	dashLen := w - 4 - len(langLabel)
	if dashLen < 0 {
		dashLen = 0
	}
	sb.WriteString(StyleDim.Render("  ╭─"+langLabel+strings.Repeat("─", dashLen)+"╮") + "\n")

	for _, line := range lines {
		visible := line
		if len(visible) > w-2 {
			visible = visible[:w-2]
		}
		sb.WriteString(StyleDim.Render("  │ ") +
			lipgloss.NewStyle().Foreground(ColorWarning).Render(visible) + "\n")
	}
	bottomDash := w - 4 + len(langLabel)
	if bottomDash < 0 {
		bottomDash = 0
	}
	sb.WriteString(StyleDim.Render("  ╰" + strings.Repeat("─", bottomDash) + "╯"))
	return sb.String()
}
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var sb strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= width {
			sb.WriteString(line)
			sb.WriteByte('\n')
			continue
		}
		for len(line) > width {
			sb.WriteString(line[:width])
			sb.WriteByte('\n')
			line = line[width:]
		}
		if len(line) > 0 {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
