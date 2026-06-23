package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/parmeet20/dockcode/concurrency"
	"github.com/parmeet20/dockcode/docker"
	"github.com/parmeet20/dockcode/llm"
	"github.com/parmeet20/dockcode/tools"
)

type AgentChunkMsg struct{ Text string }
type AgentDoneMsg struct{ Err error }
type ToolStartMsg struct {
	Name string
	Args string
}
type ToolDoneMsg struct {
	Name   string
	Result string
	Err    error
}
type AskUserMsg struct {
	Question string
	Options  []string
	Fields   []tools.AskUserField
}
type AskUserReply struct {
	Answer map[string]string
}
type SidebarRefreshMsg struct {
	Containers []docker.Container
	Images     []docker.Image
	Volumes    []docker.Volume
	Networks   []docker.Network
}
type Agent struct {
	ctx    context.Context
	cancel context.CancelFunc

	program    *tea.Program
	llm        *llm.Client
	docker     *docker.Client
	session    *Session
	memory     *Memory
	tools      *tools.Registry
	supervisor *concurrency.Supervisor

	agentBusy  *atomic.Bool
	tokenCount *atomic.Int64

	mu         sync.Mutex
	askReplyCh chan AskUserReply
}

func NewAgent(
	parent context.Context,
	llmClient *llm.Client,
	dockerClient *docker.Client,
	session *Session,
	supervisor *concurrency.Supervisor,
	agentBusy *atomic.Bool,
	tokenCount *atomic.Int64,
) *Agent {
	ctx, cancel := context.WithCancel(parent)
	a := &Agent{
		ctx:        ctx,
		cancel:     cancel,
		llm:        llmClient,
		docker:     dockerClient,
		session:    session,
		memory:     NewMemory(session),
		supervisor: supervisor,
		agentBusy:  agentBusy,
		tokenCount: tokenCount,
	}
	reg := tools.NewRegistry(dockerClient)
	a.tools = reg
	askUserTool := tools.NewAskUserTool(reg, a.askUser)
	reg.Register(tools.NewDockerCheckTool(reg))
	reg.Register(tools.NewImageListTool(reg))
	reg.Register(tools.NewImagePullTool(reg))
	reg.Register(tools.NewImageRemoveTool(reg))
	reg.Register(tools.NewContainerListTool(reg))
	reg.Register(tools.NewContainerRunTool(reg))
	reg.Register(tools.NewContainerStopTool(reg))
	reg.Register(tools.NewContainerRemoveTool(reg))
	reg.Register(tools.NewContainerLogsTool(reg))
	reg.Register(tools.NewContainerExecTool(reg))
	reg.Register(tools.NewContainerInspectTool(reg))
	reg.Register(tools.NewDockerfileWriteTool(reg))
	reg.Register(tools.NewDockerfileBuildTool(reg))
	reg.Register(tools.NewComposeWriteTool(reg))
	reg.Register(tools.NewNetworkListTool(reg))
	reg.Register(tools.NewVolumeListTool(reg))
	reg.Register(askUserTool)

	return a
}
func (a *Agent) SetProgram(p *tea.Program) {
	a.program = p
	a.llm.SetProgram(p)
}
func (a *Agent) Run(userMsg string) {
	a.agentBusy.Store(true)
	defer a.agentBusy.Store(false)
	a.session.AppendChat("user", userMsg, nil)
	pingCtx, pingCancel := context.WithTimeout(a.ctx, 5*time.Second)
	if err := a.llm.ValidateCredentials(pingCtx); err != nil {
		pingCancel()
		a.program.Send(AgentDoneMsg{
			Err: fmt.Errorf("LLM API is unreachable or misconfigured. Please check your settings with /config.\nError: %s", err.Error()),
		})
		return
	}
	pingCancel()
	history := a.session.GetChatLog()
	messages := BuildContext(a.memory, history[:len(history)-1], userMsg)
	schemas := a.tools.Schemas()

	isFirst := len(history) == 1

	for {
		select {
		case <-a.ctx.Done():
			a.program.Send(AgentDoneMsg{Err: a.ctx.Err()})
			return
		default:
		}

		deltaCh := a.llm.ChatStream(a.ctx, messages, schemas)

		dispatcher := NewStreamDispatcher(
			func(text string) {
				a.program.Send(AgentChunkMsg{Text: text})
			},
			func(id, name string) {
				a.program.Send(ToolStartMsg{Name: name})
			},
			nil,
			nil,
		)

		result := dispatcher.Run(a.ctx, deltaCh)

		if result.Error != nil {
			a.program.Send(AgentDoneMsg{Err: result.Error})
			return
		}
		a.tokenCount.Add(int64(llm.EstimateTokens(result.FullText)))
		if len(result.ToolCalls) == 0 {
			a.session.AppendChat("assistant", result.FullText, nil)
			a.program.Send(AgentDoneMsg{})
			if isFirst {
				go GenerateTitle(a.supervisor, a.ctx, a.llm, a.session, userMsg)
			}
			go a.refreshSidebar()
			return
		}
		toolResults := make([]llm.ToolResult, 0, len(result.ToolCalls))
		for _, tc := range result.ToolCalls {
			a.program.Send(ToolStartMsg{Name: tc.Name, Args: tc.Args})

			var argsRaw json.RawMessage
			if tc.Args != "" {
				argsRaw = json.RawMessage(tc.Args)
			} else {
				argsRaw = json.RawMessage(`{}`)
			}

			toolResult, err := a.tools.Dispatch(a.ctx, tc.Name, argsRaw)
			if err != nil {
				a.program.Send(ToolDoneMsg{Name: tc.Name, Err: err})
				toolResult = fmt.Sprintf("error: %s", err.Error())
			} else {
				a.program.Send(ToolDoneMsg{Name: tc.Name, Result: toolResult})
			}

			toolResults = append(toolResults, llm.ToolResult{
				ToolCallID: tc.ID,
				Content:    toolResult,
			})
		}
		messages = AppendToolRound(messages, result.FullText, result.ToolCalls, toolResults)
		select {
		case <-a.ctx.Done():
			a.program.Send(AgentDoneMsg{Err: a.ctx.Err()})
			return
		default:
		}
	}
}
func (a *Agent) askUser(ctx context.Context, args tools.AskUserArgs) (map[string]string, error) {
	ch := make(chan AskUserReply, 1)
	a.mu.Lock()
	a.askReplyCh = ch
	a.mu.Unlock()

	a.program.Send(AskUserMsg{
		Question: args.Question,
		Options:  args.Options,
		Fields:   args.Fields,
	})

	select {
	case reply := <-ch:
		return reply.Answer, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (a *Agent) SubmitAskUserReply(reply AskUserReply) {
	a.mu.Lock()
	ch := a.askReplyCh
	a.askReplyCh = nil
	a.mu.Unlock()

	if ch != nil {
		select {
		case ch <- reply:
		default:
		}
	}
}
func (a *Agent) refreshSidebar() {
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()

	var (
		containers []docker.Container
		images     []docker.Image
		volumes    []docker.Volume
		networks   []docker.Network
	)

	_ = concurrency.RunGroup(ctx, 2_000_000_000,
		func(ctx context.Context) error {
			var e error
			containers, e = a.docker.ListContainers(ctx, true)
			return e
		},
		func(ctx context.Context) error {
			var e error
			images, e = a.docker.ListImages(ctx)
			return e
		},
		func(ctx context.Context) error {
			var e error
			volumes, e = a.docker.ListVolumes(ctx)
			return e
		},
		func(ctx context.Context) error {
			var e error
			networks, e = a.docker.ListNetworks(ctx)
			return e
		},
	)

	if a.program != nil {
		a.program.Send(SidebarRefreshMsg{
			Containers: containers,
			Images:     images,
			Volumes:    volumes,
			Networks:   networks,
		})
	}
}
func (a *Agent) Stop() {
	a.cancel()
}
