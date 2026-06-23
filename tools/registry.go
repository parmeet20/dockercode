package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openai/openai-go"
	"github.com/parmeet20/dockcode/docker"
)

type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}
type Registry struct {
	tools   map[string]Tool
	Docker  *docker.Client
	Program *tea.Program
	Agent   interface{}
}

func NewRegistry(dockerClient *docker.Client) *Registry {
	return &Registry{
		tools:  make(map[string]Tool),
		Docker: dockerClient,
	}
}
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}
func (r *Registry) Dispatch(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	timeout := 60 * time.Second
	if name == "image_pull" || name == "image_build" {
		timeout = 300 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return t.Execute(ctx, args)
}
func (r *Registry) Schemas() []openai.ChatCompletionToolParam {
	schemas := make([]openai.ChatCompletionToolParam, 0, len(r.tools))
	for _, t := range r.tools {
		var params map[string]interface{}
		if len(t.Schema()) > 0 {
			_ = json.Unmarshal(t.Schema(), &params)
		}
		if params == nil {
			params = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}

		schemas = append(schemas, openai.ChatCompletionToolParam{
			Type: "function",
			Function: openai.FunctionDefinitionParam{
				Name:        t.Name(),
				Description: openai.String(t.Description()),
				Parameters:  openai.FunctionParameters(params),
			},
		})
	}
	return schemas
}
