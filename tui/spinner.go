package tui

import "time"

type Spinner struct {
	frame  int
	frames []string
}

func NewSpinner() Spinner {
	return Spinner{frames: SpinnerFrames}
}
func (s *Spinner) Tick() {
	s.frame = (s.frame + 1) % len(s.frames)
}
func (s Spinner) View() string {
	return StyleTool.Render(s.frames[s.frame])
}

const TickInterval = 80 * time.Millisecond
