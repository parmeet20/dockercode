package tui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parmeet20/dockcode/agent"
	"github.com/parmeet20/dockcode/concurrency"
	"github.com/parmeet20/dockcode/docker"
)

type SidebarPanel int

const (
	PanelContainers SidebarPanel = iota
	PanelImages
	PanelVolumes
	PanelNetworks
)

type Sidebar struct {
	activePanel SidebarPanel
	width       int
	height      int

	containers []docker.Container
	images     []docker.Image
	volumes    []docker.Volume
	networks   []docker.Network
	focused    bool
}

func NewSidebar() Sidebar                  { return Sidebar{} }
func (s *Sidebar) SetFocus(f bool)         { s.focused = f }
func (s *Sidebar) SetSize(w, h int)        { s.width = w; s.height = h }
func (s *Sidebar) SetPanel(p SidebarPanel) { s.activePanel = p }
func (s *Sidebar) Update(msg agent.SidebarRefreshMsg) {
	s.containers = msg.Containers
	s.images = msg.Images
	s.volumes = msg.Volumes
	s.networks = msg.Networks
}
func (s Sidebar) View() string {
	tabs := s.renderTabs()
	content := s.renderPanel()

	inner := tabs + "\n" + content
	style := StyleInactiveBorder
	if s.focused {
		style = StyleActiveBorder
	}
	return style.Width(s.width - 2).Height(s.height - 2).Render(inner)
}

func (s Sidebar) renderTabs() string {
	labels := []string{"[1]Cont", "[2]Img", "[3]Vol", "[4]Net"}
	var parts []string
	for i, label := range labels {
		if SidebarPanel(i) == s.activePanel {
			parts = append(parts, StyleActiveTab.Render(label))
		} else {
			parts = append(parts, StyleInactiveTab.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

func (s Sidebar) renderPanel() string {
	available := s.height - 5
	if available < 1 {
		available = 1
	}

	var lines []string
	switch s.activePanel {
	case PanelContainers:
		lines = s.renderContainers()
	case PanelImages:
		lines = s.renderImages()
	case PanelVolumes:
		lines = s.renderVolumes()
	case PanelNetworks:
		lines = s.renderNetworks()
	}

	if len(lines) == 0 {
		return StyleDim.Render("  (none)")
	}
	if len(lines) > available {
		lines = lines[:available]
	}
	return strings.Join(lines, "\n")
}

func (s Sidebar) renderContainers() []string {
	if len(s.containers) == 0 {
		return []string{StyleDim.Render("  No containers")}
	}
	var lines []string
	for _, c := range s.containers {
		status := StyleSuccess.Render("▲")
		if !strings.Contains(strings.ToLower(c.Status), "up") {
			status = StyleDim.Render("▼")
		}
		name := c.Name
		if len(name) > s.width-8 {
			name = name[:s.width-8]
		}
		lines = append(lines, fmt.Sprintf(" %s %s", status, name))
	}
	return lines
}

func (s Sidebar) renderImages() []string {
	if len(s.images) == 0 {
		return []string{StyleDim.Render("  No images")}
	}
	var lines []string
	for _, img := range s.images {
		tag := img.Repository + ":" + img.Tag
		if len(tag) > s.width-5 {
			tag = tag[:s.width-5]
		}
		size := formatSidebarSize(img.Size)
		lines = append(lines, fmt.Sprintf(" %s %s",
			StyleBase.Render(tag),
			StyleDim.Render(size),
		))
	}
	return lines
}

func (s Sidebar) renderVolumes() []string {
	if len(s.volumes) == 0 {
		return []string{StyleDim.Render("  No volumes")}
	}
	var lines []string
	for _, v := range s.volumes {
		name := v.Name
		if len(name) > s.width-5 {
			name = name[:s.width-5]
		}
		lines = append(lines, " "+StyleBase.Render(name))
	}
	return lines
}

func (s Sidebar) renderNetworks() []string {
	if len(s.networks) == 0 {
		return []string{StyleDim.Render("  No networks")}
	}
	var lines []string
	for _, n := range s.networks {
		name := n.Name
		if len(name) > s.width-5 {
			name = name[:s.width-5]
		}
		lines = append(lines, fmt.Sprintf(" %s %s",
			StyleBase.Render(name),
			StyleDim.Render(n.Driver),
		))
	}
	return lines
}

func formatSidebarSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%dKB", b>>10)
	}
}

type SidebarRefresher struct {
	docker    *docker.Client
	program   *tea.Program
	agentBusy *atomic.Bool
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewSidebarRefresher(
	parent context.Context,
	dockerClient *docker.Client,
	program *tea.Program,
	agentBusy *atomic.Bool,
) *SidebarRefresher {
	ctx, cancel := context.WithCancel(parent)
	return &SidebarRefresher{
		docker:    dockerClient,
		program:   program,
		agentBusy: agentBusy,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
}
func (r *SidebarRefresher) Start() {
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		r.refresh()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				if r.agentBusy.Load() {
					continue
				}
				r.refresh()
			}
		}
	}()
}
func (r *SidebarRefresher) Stop() {
	r.cancel()
	<-r.done
}

func (r *SidebarRefresher) refresh() {
	var (
		containers []docker.Container
		images     []docker.Image
		volumes    []docker.Volume
		networks   []docker.Network
	)
	_ = concurrency.RunGroup(r.ctx, 3*time.Second,
		func(ctx context.Context) error {
			var e error
			containers, e = r.docker.ListContainers(ctx, true)
			return e
		},
		func(ctx context.Context) error {
			var e error
			images, e = r.docker.ListImages(ctx)
			return e
		},
		func(ctx context.Context) error {
			var e error
			volumes, e = r.docker.ListVolumes(ctx)
			return e
		},
		func(ctx context.Context) error {
			var e error
			networks, e = r.docker.ListNetworks(ctx)
			return e
		},
	)

	r.program.Send(agent.SidebarRefreshMsg{
		Containers: containers,
		Images:     images,
		Volumes:    volumes,
		Networks:   networks,
	})
}

type SidebarTickMsg struct{}

func SidebarTickCmd() tea.Cmd {
	return tea.Tick(TickInterval, func(t time.Time) tea.Msg {
		return SidebarTickMsg{}
	})
}
func FormatContainerStatus(status string) string {
	lower := strings.ToLower(status)
	switch {
	case strings.Contains(lower, "up"):
		return lipgloss.NewStyle().Foreground(ColorSuccess).Render("up")
	case strings.Contains(lower, "exit"):
		return lipgloss.NewStyle().Foreground(ColorError).Render("exited")
	default:
		return lipgloss.NewStyle().Foreground(ColorDim).Render(status)
	}
}
