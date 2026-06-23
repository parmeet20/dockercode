package agent

import (
	"context"

	"github.com/parmeet20/dockcode/llm"
)

type StreamDispatcher struct {
	onText      func(text string)
	onToolStart func(id, name string)
	onToolChunk func(id, chunk string)
	onToolEnd   func(id string)
}

func NewStreamDispatcher(
	onText func(string),
	onToolStart func(id, name string),
	onToolChunk func(id, chunk string),
	onToolEnd func(id string),
) *StreamDispatcher {
	return &StreamDispatcher{
		onText:      onText,
		onToolStart: onToolStart,
		onToolChunk: onToolChunk,
		onToolEnd:   onToolEnd,
	}
}

type DispatchResult struct {
	FullText  string
	ToolCalls []llm.ToolCall
	Error     error
}

func (d *StreamDispatcher) Run(ctx context.Context, deltaCh <-chan llm.Delta) DispatchResult {
	var (
		fullText    string
		toolCalls   []llm.ToolCall
		currentTool *llm.ToolCall
	)

	for {
		select {
		case <-ctx.Done():
			return DispatchResult{FullText: fullText, ToolCalls: toolCalls, Error: ctx.Err()}
		case delta, ok := <-deltaCh:
			if !ok {
				if currentTool != nil {
					toolCalls = append(toolCalls, *currentTool)
					if d.onToolEnd != nil {
						d.onToolEnd(currentTool.ID)
					}
				}
				return DispatchResult{FullText: fullText, ToolCalls: toolCalls}
			}

			switch delta.Type {
			case "text":
				fullText += delta.Text
				if d.onText != nil {
					d.onText(delta.Text)
				}

			case "tool_call":
				currentTool = &llm.ToolCall{ID: delta.ToolID, Name: delta.ToolName}
				if d.onToolStart != nil {
					d.onToolStart(delta.ToolID, delta.ToolName)
				}

			case "tool_call_chunk":
				if currentTool != nil {
					currentTool.Args += delta.ToolArgs
					if d.onToolChunk != nil {
						d.onToolChunk(delta.ToolID, delta.ToolArgs)
					}
				}

			case "tool_call_end":
				if currentTool != nil {
					toolCalls = append(toolCalls, *currentTool)
					if d.onToolEnd != nil {
						d.onToolEnd(currentTool.ID)
					}
					currentTool = nil
				}

			case "error":
				return DispatchResult{FullText: fullText, ToolCalls: toolCalls,
					Error: &streamError{msg: delta.Text}}

			case "done":
				if currentTool != nil {
					toolCalls = append(toolCalls, *currentTool)
					if d.onToolEnd != nil {
						d.onToolEnd(currentTool.ID)
					}
				}
				return DispatchResult{FullText: fullText, ToolCalls: toolCalls}
			}
		}
	}
}

type streamError struct{ msg string }

func (e *streamError) Error() string { return e.msg }
