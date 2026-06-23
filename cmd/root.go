package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/parmeet20/dockcode/agent"
	"github.com/parmeet20/dockcode/concurrency"
	"github.com/parmeet20/dockcode/config"
	"github.com/parmeet20/dockcode/docker"
	"github.com/parmeet20/dockcode/llm"
	"github.com/parmeet20/dockcode/tui"
)

var rootCmd = &cobra.Command{
	Use:   "dockcode",
	Short: "🐳 DockCode — AI-powered Docker management TUI",
	RunE: func(cmd *cobra.Command, args []string) error {
		return startApp(cmd.Context())
	},
}

func Execute(ctx context.Context) error {
	_ = EnsureGoBinInPath()
	return rootCmd.ExecuteContext(ctx)
}

func startApp(ctx context.Context) error {
	cfgManager, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if !cfgManager.ConfigExists() {
		return runOnboarding(ctx, cfgManager)
	}

	if err := cfgManager.Load(); err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg := cfgManager.Get()
	dockerClient, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	llmClient := llm.NewClient(cfg.APIURL, cfg.APIToken, cfg.Model)
	home, _ := os.UserHomeDir()
	sessionsDir := filepath.Join(home, ".dockcode", "sessions")
	_ = os.MkdirAll(sessionsDir, 0755)

	sessIdx, err := agent.NewSessionIndex(sessionsDir)
	if err != nil {
		return fmt.Errorf("session index: %w", err)
	}

	sess, err := agent.NewSession(ctx, sessionsDir)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	_ = sessIdx.Upsert(agent.SessionSummary{
		ID:        sess.ID,
		Title:     "New Session",
		UpdatedAt: time.Now().Format(time.RFC3339),
	})
	sup := concurrency.NewSupervisor()
	appCtx, appCancel := context.WithCancel(ctx)
	m := tui.NewModel(appCtx, appCancel, cfgManager, dockerClient, llmClient, sess, sessIdx, sup)
	p := tea.NewProgram(
		&m,
		tea.WithAltScreen(),
	)
	m.SetProgram(p)
	var _ atomic.Bool

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	return nil
}

func runOnboarding(ctx context.Context, cfgManager *config.Manager) error {
	m := tui.NewOnboardingModel(ctx, cfgManager)
	p := tea.NewProgram(m, tea.WithAltScreen())

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("onboarding: %w", err)
	}
	if cfgManager.ConfigExists() {
		return startApp(ctx)
	}
	return nil
}
