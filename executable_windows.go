//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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
	return resolveExecutablePathWith(value, resolveExecutableWithWhere)
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

func resolveExecutableWithWhere(executable string) string {
	wherePath := "where.exe"
	if systemRoot := strings.TrimSpace(os.Getenv("SystemRoot")); systemRoot != "" {
		candidate := filepath.Join(systemRoot, "System32", "where.exe")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			wherePath = candidate
		}
	}

	command := exec.Command(wherePath, executable)
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	output, err := command.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.FieldsFunc(string(output), func(r rune) bool {
		return r == '\r' || r == '\n'
	}) {
		if candidate := strings.TrimSpace(line); candidate != "" {
			return candidate
		}
	}
	return ""
}
