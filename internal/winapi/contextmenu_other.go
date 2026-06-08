//go:build !windows

package winapi

func SetCreateIgnoreContextMenu(exePath string, enabled bool) error {
	return nil
}
