package agent

import (
	"fmt"
	"strings"
)

type Memory struct {
	session *Session
}

func NewMemory(s *Session) *Memory {
	return &Memory{session: s}
}
func (m *Memory) GetSection(section string) string {
	md := m.session.GetAgentMD()
	lines := strings.Split(md, "\n")
	heading := "## " + section
	var buf strings.Builder
	inSection := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if inSection {
				break
			}
			if line == heading {
				inSection = true
				continue
			}
		}
		if inSection {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	return strings.TrimSpace(buf.String())
}
func (m *Memory) SetSection(section, content string) {
	md := m.session.GetAgentMD()
	heading := "## " + section
	lines := strings.Split(md, "\n")

	var result strings.Builder
	inSection := false
	sectionFound := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if inSection {
				result.WriteString(content)
				result.WriteString("\n\n")
				inSection = false
			}
			if line == heading {
				inSection = true
				sectionFound = true
				result.WriteString(line)
				result.WriteByte('\n')
				continue
			}
		}
		if !inSection {
			result.WriteString(line)
			result.WriteByte('\n')
		}
	}
	if inSection {
		result.WriteString(content)
		result.WriteString("\n\n")
	}
	if !sectionFound {
		result.WriteString(fmt.Sprintf("\n## %s\n%s\n", section, content))
	}

	m.session.UpdateAgentMD(result.String())
}
func (m *Memory) AppendToSection(section, content string) {
	existing := m.GetSection(section)
	if existing == "" {
		m.SetSection(section, content)
	} else {
		m.SetSection(section, existing+"\n"+content)
	}
}
func (m *Memory) BuildSystemPrompt() string {
	md := m.session.GetAgentMD()
	return fmt.Sprintf(`You are DockCode, an expert AI assistant for managing Docker through natural language.
You have access to Docker tools to inspect, create, stop, and manage containers and images.
Always call docker_status first. Use ask_user when you need clarification before running commands.

%s`, md)
}
