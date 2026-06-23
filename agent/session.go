package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SessionMeta struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TokensIn  int       `json:"tokens_in"`
	TokensOut int       `json:"tokens_out"`
	Tags      []string  `json:"tags"`
}
type ChatEntry struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}
type Session struct {
	mu      sync.RWMutex
	ID      string
	Dir     string
	Meta    SessionMeta
	agentMD string
	chatLog []ChatEntry
	dirty   bool
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewSession(parent context.Context, baseDir string) (*Session, error) {
	id := fmt.Sprintf("session-%d", time.Now().UnixNano())
	dir := filepath.Join(baseDir, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session dir: %w", err)
	}

	ctx, cancel := context.WithCancel(parent)
	s := &Session{
		ID:  id,
		Dir: dir,
		Meta: SessionMeta{
			ID:        id,
			Title:     "New Session",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Tags:      []string{},
		},
		agentMD: fmt.Sprintf("# Agent Memory — %s\n\n## Context\nNew session.\n\n## Docker Status\nUnknown.\n", id),
		chatLog: []ChatEntry{},
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	if err := s.Save(); err != nil {
		cancel()
		return nil, err
	}

	go s.autoSave()
	return s, nil
}
func LoadSession(parent context.Context, dir string) (*Session, error) {
	ctx, cancel := context.WithCancel(parent)
	s := &Session{
		Dir:    dir,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	metaData, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to read meta.json: %w", err)
	}
	if err := json.Unmarshal(metaData, &s.Meta); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to parse meta.json: %w", err)
	}
	s.ID = s.Meta.ID
	agentData, err := os.ReadFile(filepath.Join(dir, "agent.md"))
	if err == nil {
		s.agentMD = string(agentData)
	}
	chatData, err := os.ReadFile(filepath.Join(dir, "chat.md"))
	if err == nil {
		s.chatLog = parseChatMD(string(chatData))
	}

	go s.autoSave()
	return s, nil
}
func (s *Session) autoSave() {
	defer close(s.done)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			dirty := s.dirty
			s.mu.RUnlock()
			if dirty {
				_ = s.Save()
			}
		}
	}
}
func (s *Session) AppendChat(role, content string, toolCalls interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatLog = append(s.chatLog, ChatEntry{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	s.Meta.UpdatedAt = time.Now()
	s.dirty = true
}
func (s *Session) UpdateAgentMD(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentMD = content
	s.dirty = true
}
func (s *Session) GetAgentMD() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agentMD
}
func (s *Session) GetChatLog() []ChatEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ChatEntry, len(s.chatLog))
	copy(out, s.chatLog)
	return out
}
func (s *Session) SetTitle(title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Meta.Title = title
	s.Meta.UpdatedAt = time.Now()
	s.dirty = true
}
func (s *Session) GetMeta() SessionMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Meta
}
func (s *Session) Save() error {
	s.mu.RLock()
	meta := s.Meta
	agentMD := s.agentMD
	chatLog := make([]ChatEntry, len(s.chatLog))
	copy(chatLog, s.chatLog)
	s.mu.RUnlock()
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(s.Dir, "meta.json"), metaData); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(s.Dir, "agent.md"), []byte(agentMD)); err != nil {
		return err
	}
	chatMD := formatChatMD(chatLog)
	if err := atomicWrite(filepath.Join(s.Dir, "chat.md"), []byte(chatMD)); err != nil {
		return err
	}

	s.mu.Lock()
	s.dirty = false
	s.mu.Unlock()

	return nil
}
func (s *Session) Stop() {
	s.cancel()
	<-s.done
}
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func formatChatMD(log []ChatEntry) string {
	var sb strings.Builder
	sb.WriteString("# Chat Log\n\n")
	for _, e := range log {
		sb.WriteString(fmt.Sprintf("## %s [%s]\n\n%s\n\n---\n\n",
			strings.ToUpper(e.Role), e.Timestamp.Format(time.RFC3339), e.Content))
	}
	return sb.String()
}
func parseChatMD(content string) []ChatEntry {
	var log []ChatEntry
	blocks := strings.Split(content, "\n\n---\n\n")
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || !strings.HasPrefix(block, "## ") {
			continue
		}
		lines := strings.SplitN(block, "\n", 2)
		if len(lines) < 2 {
			continue
		}
		header := lines[0]
		body := strings.TrimSpace(lines[1])
		header = strings.TrimPrefix(header, "## ")
		parts := strings.SplitN(header, " [", 2)
		role := strings.ToLower(strings.TrimSpace(parts[0]))

		var timestamp time.Time
		if len(parts) > 1 {
			timeStr := strings.TrimSuffix(parts[1], "]")
			if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
				timestamp = t
			} else {
				timestamp = time.Now()
			}
		} else {
			timestamp = time.Now()
		}

		log = append(log, ChatEntry{
			Role:      role,
			Content:   body,
			Timestamp: timestamp,
		})
	}
	return log
}
