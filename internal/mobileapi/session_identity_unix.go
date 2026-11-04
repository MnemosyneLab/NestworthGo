//go:build darwin || linux

package mobileapi

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func databaseFileIdentity(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return "", errSessionRegistry
	}
	return fmt.Sprintf("%s:%d:%d", resolved, stat.Dev, stat.Ino), nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
