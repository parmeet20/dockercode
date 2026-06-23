//go:build windows

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

func EnsureGoBinInPath() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	goBin := filepath.Join(home, "go", "bin")
	currentPath := os.Getenv("PATH")
	if isInPath(currentPath, goBin) {
		return nil
	}
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		"Environment",
		registry.QUERY_VALUE|registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("path setup: open registry: %w", err)
	}
	defer k.Close()

	regVal, valType, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("path setup: read registry Path: %w", err)
	}
	if isInPath(regVal, goBin) {
		_ = os.Setenv("PATH", currentPath+string(os.PathListSeparator)+goBin)
		return nil
	}
	newVal := regVal
	if newVal != "" && !strings.HasSuffix(newVal, ";") {
		newVal += ";"
	}
	newVal += goBin
	if valType == registry.EXPAND_SZ || strings.Contains(newVal, "%") {
		err = k.SetExpandStringValue("Path", newVal)
	} else {
		err = k.SetStringValue("Path", newVal)
	}
	if err != nil {
		return fmt.Errorf("path setup: write registry: %w", err)
	}
	_ = os.Setenv("PATH", currentPath+string(os.PathListSeparator)+goBin)
	broadcastEnvChange()

	return nil
}
func isInPath(pathEnv, target string) bool {
	for _, p := range filepath.SplitList(pathEnv) {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(target)) {
			return true
		}
	}
	return false
}
func broadcastEnvChange() {
	const (
		hwndBroadcast   = uintptr(0xFFFF)
		wmSettingChange = uintptr(0x001A)
		smtoAbortIfHung = uintptr(0x0002)
		timeoutMs       = 5000
	)
	user32 := syscall.NewLazyDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")
	envStr, _ := syscall.UTF16PtrFromString("Environment")
	var result uintptr
	_, _, _ = sendMessageTimeout.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(envStr)),
		smtoAbortIfHung,
		timeoutMs,
		uintptr(unsafe.Pointer(&result)),
	)
}
