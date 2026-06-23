package llm

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/pagination"
	"github.com/openai/openai-go/packages/ssestream"
)

type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Multiplier  float64
}
type Client struct {
	mu       sync.RWMutex
	baseURL  string
	token    string
	model    string
	client   *openai.Client
	program  *tea.Program
	retryCfg RetryConfig
}
type Delta struct {
	Type     string
	Text     string
	ToolName string
	ToolArgs string
	ToolID   string
}
type RetryMsg struct {
	Message string
	Error   bool
}

func NewClient(baseURL, token, model string) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		model:   model,
		retryCfg: RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   1 * time.Second,
			MaxDelay:    30 * time.Second,
			Multiplier:  2.0,
		},
	}
	c.reinitClient()
	return c
}
func (c *Client) SetProgram(p *tea.Program) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.program = p
}
func (c *Client) UpdateConfig(baseURL, token, model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = strings.TrimRight(baseURL, "/")
	c.token = token
	c.model = model
	c.reinitClient()
}
func (c *Client) GetModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

func (c *Client) reinitClient() {
	client := openai.NewClient(
		option.WithAPIKey(c.token),
		option.WithBaseURL(c.baseURL),
	)
	c.client = &client
}

func (c *Client) notify(msg string, isError bool) {
	c.mu.RLock()
	p := c.program
	c.mu.RUnlock()
	if p != nil {
		p.Send(RetryMsg{Message: msg, Error: isError})
	}
}
func (c *Client) withRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	c.mu.RLock()
	cfg := c.retryCfg
	base := c.baseURL
	c.mu.RUnlock()

	delay := cfg.BaseDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			code := apiErr.StatusCode
			if code == 401 {
				msg := "✖ Error  Invalid API token. Run /settoken to update it."
				c.notify(msg, true)
				return errors.New(msg)
			} else if code == 403 {
				msg := "✖ Error  Access denied. Check your token permissions."
				c.notify(msg, true)
				return errors.New(msg)
			} else if code == 400 {
				msg := fmt.Sprintf("✖ Error  Bad request: %s", apiErr.Error())
				c.notify(msg, true)
				return errors.New(msg)
			} else if code == 429 {
				msg := fmt.Sprintf("◆ Info  Rate limited. Retrying in %v... (attempt %d/%d)", delay, attempt, cfg.MaxAttempts)
				c.notify(msg, false)
				lastErr = err
			} else if code >= 500 {
				msg := fmt.Sprintf("◆ Info  API error %d. Retrying in %v... (attempt %d/%d)", code, delay, attempt, cfg.MaxAttempts)
				c.notify(msg, false)
				lastErr = err
			} else {
				lastErr = err
			}
		} else {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			msg := fmt.Sprintf("◆ Info  Connection failed. Retrying in %v...", delay)
			c.notify(msg, false)
			lastErr = err
		}

		if attempt == cfg.MaxAttempts {
			break
		}

		jitter := time.Duration(rand.Int63n(int64(delay / 4)))
		select {
		case <-time.After(delay + jitter):
		case <-ctx.Done():
			return ctx.Err()
		}
		delay = time.Duration(float64(delay) * cfg.Multiplier)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}

	errMsg := lastErr.Error()
	if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "no such host") || strings.Contains(errMsg, "dial tcp") {
		msg := fmt.Sprintf("✖ Error  Cannot connect to %s. Is this correct?\n          Run /seturl to change it. Common values:\n          • https://api.openai.com/v1\n          • http://localhost:11434/v1  (Ollama)\n          • https://api.groq.com/openai/v1", base)
		c.notify(msg, true)
		return errors.New(msg)
	}

	msg := fmt.Sprintf("✖ Error  Could not reach API after %d attempts. Check your base URL with /seturl", cfg.MaxAttempts)
	c.notify(msg, true)
	return errors.New(msg)
}
func (c *Client) ChatStream(ctx context.Context, messages []Message, tools []openai.ChatCompletionToolParam) <-chan Delta {
	deltaCh := make(chan Delta, 32)

	go func() {
		defer close(deltaCh)

		c.mu.RLock()
		modelName := c.model
		sdkClient := c.client
		c.mu.RUnlock()

		sdkMessages := mapMessagesToParam(messages)

		params := openai.ChatCompletionNewParams{
			Messages: sdkMessages,
			Model:    openai.ChatModel(modelName),
		}
		if len(tools) > 0 {
			params.Tools = tools
		}

		var stream *ssestream.Stream[openai.ChatCompletionChunk]
		err := c.withRetry(ctx, func() error {
			stream = sdkClient.Chat.Completions.NewStreaming(ctx, params)
			return stream.Err()
		})

		if err != nil {
			select {
			case deltaCh <- Delta{Type: "error", Text: err.Error()}:
			case <-ctx.Done():
			}
			return
		}
		defer stream.Close()

		var activeToolCall *ToolCall
		for stream.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			if choice.Delta.Content != "" {
				select {
				case deltaCh <- Delta{Type: "text", Text: choice.Delta.Content}:
				case <-ctx.Done():
					return
				}
			}

			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					if tc.ID != "" {
						if activeToolCall != nil {
							select {
							case deltaCh <- Delta{Type: "tool_call_end", ToolID: activeToolCall.ID}:
							case <-ctx.Done():
								return
							}
						}
						activeToolCall = &ToolCall{
							ID:   tc.ID,
							Name: tc.Function.Name,
							Args: tc.Function.Arguments,
						}
						select {
						case deltaCh <- Delta{Type: "tool_call", ToolID: tc.ID, ToolName: tc.Function.Name}:
						case <-ctx.Done():
							return
						}
						if tc.Function.Arguments != "" {
							select {
							case deltaCh <- Delta{Type: "tool_call_chunk", ToolID: tc.ID, ToolArgs: tc.Function.Arguments}:
							case <-ctx.Done():
								return
							}
						}
					} else {
						if activeToolCall != nil && tc.Function.Arguments != "" {
							activeToolCall.Args += tc.Function.Arguments
							select {
							case deltaCh <- Delta{Type: "tool_call_chunk", ToolID: activeToolCall.ID, ToolArgs: tc.Function.Arguments}:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}

		if activeToolCall != nil {
			select {
			case deltaCh <- Delta{Type: "tool_call_end", ToolID: activeToolCall.ID}:
			case <-ctx.Done():
				return
			}
		}

		if err := stream.Err(); err != nil {
			select {
			case deltaCh <- Delta{Type: "error", Text: err.Error()}:
			case <-ctx.Done():
			}
			return
		}

		select {
		case deltaCh <- Delta{Type: "done"}:
		case <-ctx.Done():
		}
	}()

	return deltaCh
}
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	c.mu.RLock()
	sdkClient := c.client
	c.mu.RUnlock()

	var response *pagination.Page[openai.Model]
	err := c.withRetry(ctx, func() error {
		var err error
		response, err = sdkClient.Models.List(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(response.Data))
	for _, m := range response.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}
func (c *Client) ValidateCredentials(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.ListModels(ctx)
	return err
}

func mapMessagesToParam(messages []Message) []openai.ChatCompletionMessageParamUnion {
	params := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system":
			params = append(params, openai.SystemMessage(m.Content))
		case "user":
			params = append(params, openai.UserMessage(m.Content))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				sdkToolCalls := make([]openai.ChatCompletionMessageToolCallParam, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					sdkToolCalls = append(sdkToolCalls, openai.ChatCompletionMessageToolCallParam{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Args,
						},
					})
				}
				params = append(params, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Role:      "assistant",
						Content:   openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(m.Content)},
						ToolCalls: sdkToolCalls,
					},
				})
			} else {
				params = append(params, openai.AssistantMessage(m.Content))
			}
		case "tool":
			params = append(params, openai.ToolMessage(m.Content, m.ToolCallID))
		}
	}
	return params
}
