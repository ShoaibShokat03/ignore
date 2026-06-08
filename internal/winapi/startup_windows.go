//go:build windows

package winapi

import (
	"errors"
	"os/exec"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func SetStartup(name, exePath string, enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		err := key.DeleteValue(name)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	// Launch with --background so a login start comes up hidden in the tray
	// (protection running) instead of popping the window open.
	return key.SetStringValue(name, strconv.Quote(exePath)+" --background")
}

func OpenPath(path string) error {
	return exec.Command("explorer.exe", path).Start()
}
