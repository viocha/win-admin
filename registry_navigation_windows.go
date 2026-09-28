//go:build windows

package main

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func openRegistryKey(registryPath string) error {
	const regeditStatePath = `Software\Microsoft\Windows\CurrentVersion\Applets\Regedit`
	state, _, err := registry.CreateKey(registry.CURRENT_USER, regeditStatePath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("设置注册表编辑器位置失败: %w", err)
	}
	lastKey, _, _ := state.GetStringValue("LastKey")
	computer := "Computer"
	if prefix, _, found := strings.Cut(lastKey, `\`); found && prefix != "" {
		computer = prefix
	}
	if err := state.SetStringValue("LastKey", computer+`\`+registryPath); err != nil {
		state.Close()
		return fmt.Errorf("设置注册表编辑器位置失败: %w", err)
	}
	state.Close()
	program, _ := windows.UTF16PtrFromString("regedit.exe")
	parameters, _ := windows.UTF16PtrFromString("/m")
	if err := windows.ShellExecute(0, nil, program, parameters, nil, 1); err != nil {
		return fmt.Errorf("打开注册表编辑器失败: %w", err)
	}
	return nil
}
