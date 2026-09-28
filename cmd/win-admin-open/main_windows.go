//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"win-admin/internal/fileassociation"
)

const (
	associationClasses         = `Software\Classes\`
	associationCommandValue    = "WinAdminCommand"
	associationExecutableValue = "WinAdminExecutable"
)

var associationExtensionPattern = regexp.MustCompile(`^\.[a-z0-9][a-z0-9_-]*$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		showError(err)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return errors.New("文件打开参数无效")
	}

	extension := strings.ToLower(strings.TrimSpace(args[0]))
	if !associationExtensionPattern.MatchString(extension) {
		return errors.New("文件扩展名无效")
	}

	filePath, err := filepath.Abs(args[1])
	if err != nil {
		return fmt.Errorf("读取文件路径失败: %w", err)
	}
	if !strings.EqualFold(filepath.Ext(filePath), extension) {
		return errors.New("文件扩展名与打开方式不匹配")
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, associationClasses+associationProgID(extension), registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return errors.New("该扩展名的打开方式不存在")
		}
		return fmt.Errorf("读取打开命令失败: %w", err)
	}
	command, _, commandErr := key.GetStringValue(associationCommandValue)
	executable, _, executableErr := key.GetStringValue(associationExecutableValue)
	key.Close()
	if commandErr != nil {
		return fmt.Errorf("读取打开命令失败: %w", commandErr)
	}
	if executableErr != nil {
		return fmt.Errorf("读取程序路径失败: %w", executableErr)
	}

	parts, err := commandParts(command, filePath)
	if err != nil {
		return err
	}
	if err := startProcess(executable, parts[1:], filepath.Dir(filePath)); err != nil {
		return fmt.Errorf("运行打开命令失败: %w", err)
	}
	return nil
}

func startProcess(executable string, args []string, dir string) error {
	appName, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	commandLine, err := windows.UTF16FromString(windows.ComposeCommandLine(append([]string{executable}, args...)))
	if err != nil {
		return err
	}
	currentDir, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}

	startupInfo := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var processInfo windows.ProcessInformation
	if err := windows.CreateProcess(
		appName,
		&commandLine[0],
		nil,
		nil,
		false,
		windows.CREATE_NEW_CONSOLE,
		nil,
		currentDir,
		&startupInfo,
		&processInfo,
	); err != nil {
		return err
	}
	_ = windows.CloseHandle(processInfo.Thread)
	_ = windows.CloseHandle(processInfo.Process)
	return nil
}

func associationProgID(extension string) string {
	return "WinAdmin.File." + strings.TrimPrefix(extension, ".")
}

func commandParts(command, filePath string) ([]string, error) {
	return fileassociation.CommandParts(command, filePath)
}

func showError(err error) {
	message, _ := windows.UTF16PtrFromString(err.Error())
	title, _ := windows.UTF16PtrFromString("打开文件失败")
	_, _ = windows.MessageBox(0, message, title, 0x10)
}
