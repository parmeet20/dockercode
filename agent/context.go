package agent

import (
	"github.com/parmeet20/dockcode/llm"
)

func BuildContext(mem *Memory, history []ChatEntry, userMsg string) []llm.Message {
	messages := []llm.Message{
		{Role: "system", Content: mem.BuildSystemPrompt()},
	}

	for _, e := range history {
		switch e.Role {
		case "user", "assistant":
			messages = append(messages, llm.Message{
				Role:    e.Role,
				Content: e.Content,
			})
		case "tool":
			messages = append(messages, llm.Message{
				Role:    "tool",
				Content: e.Content,
			})
		}
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userMsg,
	})

	return messages
}
func AppendToolRound(
	messages []llm.Message,
	assistantText string,
	toolCalls []llm.ToolCall,
	results []llm.ToolResult,
) []llm.Message {
	messages = append(messages, llm.Message{
		Role:      "assistant",
		Content:   assistantText,
		ToolCalls: toolCalls,
	})

	for _, r := range results {
		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    r.Content,
			ToolCallID: r.ToolCallID,
		})
	}
	return messages
}
