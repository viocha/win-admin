//go:build windows

package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

func commandExecutableIsPath(executable string) bool {
	return filepath.IsAbs(executable) ||
		strings.ContainsAny(executable, `\\/`) ||
		strings.Contains(executable, ":") ||
		strings.Contains(executable, "%")
}

func quoteCommandExecutable(executable string) string {
	if strings.ContainsAny(executable, " \t") {
		return `"` + executable + `"`
	}
	return executable
}

func resolveExecutablePath(value string) string {
	return resolveExecutablePathWith(value, lookupExecutablePath)
}

func resolveExecutablePathWith(value string, lookup func(string) string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	if value == "" || commandExecutableIsPath(value) {
		return value
	}

	resolved := strings.TrimSpace(lookup(value))
	if resolved == "" {
		return value
	}
	return resolved
}

func lookupExecutablePath(executable string) string {
	// 显式查找当前目录，保留 where 的解析顺序；LookPath 负责 PATHEXT。
	if current, err := filepath.Abs(executable); err == nil {
		if resolved, err := exec.LookPath(current); err == nil {
			return resolved
		}
	}
	resolved, err := exec.LookPath(executable)
	if err != nil && !errors.Is(err, exec.ErrDot) {
		return ""
	}
	if resolved == "" {
		return ""
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return ""
	}
	return absolute
}
