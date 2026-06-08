package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"ignore/internal/config"
)

func RunCommandLine() (bool, error) {
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch strings.ToLower(args[i]) {
		case "--create-ignore", "/create-ignore", "-create-ignore":
			target := ""
			if i+1 < len(args) {
				target = args[i+1]
			}
			_, err := CreateIgnoreFile(target)
			return true, err
		}
	}
	return false, nil
}

func CreateIgnoreFile(target string) (string, error) {
	if strings.TrimSpace(target) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		target = cwd
	}
	target = strings.Trim(target, `"`)
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		target = filepath.Dir(target)
	}
	path := filepath.Join(target, ".ignore")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, os.WriteFile(path, []byte(config.DefaultIgnoreContent()), 0o644)
}
