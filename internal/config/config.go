package config

import (
	"os"
	"path/filepath"
)

const EnvDBPath = "COREMEM_DB_PATH"

func DBPath() (string, error) {
	if p := os.Getenv(EnvDBPath); p != "" {
		return expandHome(p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".coremem", "coremem.db"), nil
}

func expandHome(path string) (string, error) {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if len(path) > 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
