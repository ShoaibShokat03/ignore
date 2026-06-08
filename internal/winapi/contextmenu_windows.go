//go:build windows

package winapi

import (
	"errors"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const (
	folderShellKey     = `Software\Classes\Directory\shell\IgnoreCreateIgnore`
	backgroundShellKey = `Software\Classes\Directory\Background\shell\IgnoreCreateIgnore`
)

func SetCreateIgnoreContextMenu(exePath string, enabled bool) error {
	if !enabled {
		if err := deleteRegistryTree(registry.CURRENT_USER, folderShellKey); err != nil {
			return err
		}
		return deleteRegistryTree(registry.CURRENT_USER, backgroundShellKey)
	}
	if err := writeCreateIgnoreMenu(folderShellKey, exePath, "%1"); err != nil {
		return err
	}
	return writeCreateIgnoreMenu(backgroundShellKey, exePath, "%V")
}

func writeCreateIgnoreMenu(keyPath, exePath, targetArg string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	if err := key.SetStringValue("", "Create .ignore file"); err != nil {
		key.Close()
		return err
	}
	_ = key.SetStringValue("Icon", exePath+",0")
	key.Close()

	cmdKey, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath+`\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer cmdKey.Close()
	command := strconv.Quote(exePath) + " --create-ignore " + strconv.Quote(targetArg)
	return cmdKey.SetStringValue("", command)
}

func deleteRegistryTree(root registry.Key, path string) error {
	if err := registry.DeleteKey(root, path+`\command`); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	if err := registry.DeleteKey(root, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
