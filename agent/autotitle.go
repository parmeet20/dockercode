package agent

import (
	"context"
	"strings"
	"time"

	"github.com/parmeet20/dockcode/concurrency"
	"github.com/parmeet20/dockcode/llm"
)

func GenerateTitle(
	sup *concurrency.Supervisor,
	parent context.Context,
	llmClient *llm.Client,
	session *Session,
	firstUserMsg string,
) {
	sup.Go(parent, "autotitle", func() {
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()

		messages := []llm.Message{
			{
				Role:    "user",
				Content: "Generate a title of 5 words maximum for this conversation. Reply with ONLY the title, no quotes, no punctuation at the end.\n\nConversation: " + firstUserMsg,
			},
		}

		deltaCh := llmClient.ChatStream(ctx, messages, nil)

		var title strings.Builder
		for d := range deltaCh {
			if d.Type == "text" {
				title.WriteString(d.Text)
			}
			if d.Type == "done" || d.Type == "error" {
				break
			}
		}

		t := strings.TrimSpace(title.String())
		if t == "" || len(t) > 80 {
			t = firstUserMsg
			if len(t) > 40 {
				t = t[:40] + "…"
			}
		}
		session.SetTitle(t)
	})
}
