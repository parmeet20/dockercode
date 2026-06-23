package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type SessionSummary struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	UpdatedAt string   `json:"updated_at"`
	TokensIn  int      `json:"tokens_in"`
	TokensOut int      `json:"tokens_out"`
	Tags      []string `json:"tags"`
}
type SessionIndex struct {
	mu      sync.RWMutex
	path    string
	entries []SessionSummary
}

func NewSessionIndex(baseDir string) (*SessionIndex, error) {
	path := filepath.Join(baseDir, "index.json")
	idx := &SessionIndex{path: path}
	if err := idx.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return idx, nil
}

func (i *SessionIndex) load() error {
	data, err := os.ReadFile(i.path)
	if err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return json.Unmarshal(data, &i.entries)
}
func (i *SessionIndex) Upsert(s SessionSummary) error {
	i.mu.Lock()
	found := false
	for j, e := range i.entries {
		if e.ID == s.ID {
			i.entries[j] = s
			found = true
			break
		}
	}
	if !found {
		i.entries = append(i.entries, s)
	}
	entries := make([]SessionSummary, len(i.entries))
	copy(entries, i.entries)
	i.mu.Unlock()

	sort.Slice(entries, func(a, b int) bool {
		return entries[a].UpdatedAt > entries[b].UpdatedAt
	})

	i.mu.Lock()
	i.entries = entries
	i.mu.Unlock()

	return i.save()
}
func (i *SessionIndex) Delete(id string) error {
	i.mu.Lock()
	out := i.entries[:0]
	for _, e := range i.entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	i.entries = out
	i.mu.Unlock()
	return i.save()
}
func (i *SessionIndex) List() []SessionSummary {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]SessionSummary, len(i.entries))
	copy(out, i.entries)
	return out
}

func (i *SessionIndex) save() error {
	i.mu.RLock()
	data, err := json.MarshalIndent(i.entries, "", "  ")
	i.mu.RUnlock()
	if err != nil {
		return err
	}
	return atomicWrite(i.path, data)
}
