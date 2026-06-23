//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func EnsureGoBinInPath() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	goBin := filepath.Join(home, "go", "bin")
	pathEnv := os.Getenv("PATH")
	paths := filepath.SplitList(pathEnv)
	for _, p := range paths {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(goBin)) {
			return nil
		}
	}
	var shellConfigs []string
	if runtime.GOOS == "darwin" {
		shellConfigs = []string{".zshrc", ".bash_profile", ".profile"}
	} else {
		shellConfigs = []string{".zshrc", ".bashrc", ".profile"}
	}

	exportCmd := fmt.Sprintf("\n# Added by DockCode\nexport PATH=\"$PATH:%s\"\n", goBin)

	for _, cfg := range shellConfigs {
		p := filepath.Join(home, cfg)
		if _, err := os.Stat(p); err == nil {
			content, err := os.ReadFile(p)
			if err == nil && !strings.Contains(string(content), goBin) {
				f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
				if err == nil {
					_, _ = f.WriteString(exportCmd)
					_ = f.Close()
				}
			}
		}
	}

	return nil
}
