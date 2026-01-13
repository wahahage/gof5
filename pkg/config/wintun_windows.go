//go:build windows
// +build windows

package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const (
	winTun     = "wintun.dll"
	winTunSite = "https://www.wintun.net/"
)

//go:embed wintun.dll
var wintunContent []byte

func checkWinTunDriver() error {
	// Try to load the DLL first to check if it's already available
	err := windows.NewLazyDLL(winTun).Load()
	if err == nil {
		return nil
	}

	// If loading failed, try to write the embedded DLL to the executable's directory
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}
	dir := filepath.Dir(exePath)
	dllPath := filepath.Join(dir, winTun)

	// Check if file already exists (maybe it exists but load failed for another reason, or it's missing)
	// We'll try to write it if it doesn't exist or if we want to force update (optional, simple logic: write if missing)
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		if err := os.WriteFile(dllPath, wintunContent, 0644); err != nil {
			return fmt.Errorf("failed to write embedded %s: %v", winTun, err)
		}
	}

	// Try loading again
	err = windows.NewLazyDLL(winTun).Load()
	if err != nil {
		return fmt.Errorf("the %s was not found and could not be extracted. You can download it from %s and place it into the %q directory", winTun, winTunSite, dir)
	}

	return nil
}
