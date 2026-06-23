package tools

import (
	"context"
	"encoding/json"
)

type AskUserField struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Secret  bool   `json:"secret"`
	Default string `json:"default"`
}
type AskUserArgs struct {
	Question string         `json:"question"`
	Options  []string       `json:"options"`
	Fields   []AskUserField `json:"fields"`
}
type AskUserCallback func(ctx context.Context, args AskUserArgs) (map[string]string, error)
type AskUserTool struct {
	r        *Registry
	callback AskUserCallback
}

func NewAskUserTool(r *Registry, cb AskUserCallback) *AskUserTool {
	return &AskUserTool{r: r, callback: cb}
}

func (t *AskUserTool) Name() string { return "ask_user" }
func (t *AskUserTool) Description() string {
	return "Ask the user a clarifying question before proceeding. " +
		"Use this to gather: image version, port mappings, environment variable values, " +
		"container name, passwords. Always call this BEFORE any container_run or image_pull " +
		"if any parameter is unknown."
}
func (t *AskUserTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "The question to display to the user"
			},
			"options": {
				"type": "array",
				"items": {"type": "string"},
				"description": "If provided, show as selectable options. If empty, show free text input."
			},
			"fields": {
				"type": "array",
				"description": "If provided, show a multi-field form",
				"items": {
					"type": "object",
					"properties": {
						"name":    {"type": "string"},
						"label":   {"type": "string"},
						"secret":  {"type": "boolean"},
						"default": {"type": "string"}
					},
					"required": ["name", "label"]
				}
			}
		},
		"required": ["question"]
	}`)
}

func (t *AskUserTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a AskUserArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	reply, err := t.callback(ctx, a)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(reply)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
