//go:build !windows

package winapi

import "errors"

func SetStartup(name, exePath string, enabled bool) error {
	return errors.New("startup registration is only supported on Windows")
}

func OpenPath(path string) error {
	return errors.New("opening paths is only supported on Windows")
}
