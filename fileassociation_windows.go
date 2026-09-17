//go:build windows

package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	associationAppName         = "win-admin"
	associationCapabilities    = `Software\win-admin\Capabilities`
	associationMappings        = associationCapabilities + `\FileAssociations`
	associationClasses         = `Software\Classes\`
	associationUserChoice      = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`
	associationCommandValue    = "WinAdminCommand"
	associationExecutableValue = "WinAdminExecutable"
)

var associationExtensionPattern = regexp.MustCompile(`^\.[a-z0-9][a-z0-9_-]*$`)

func normalizeAssociationExtension(value string) (string, error) {
	extension := strings.ToLower(strings.TrimSpace(value))
	if extension != "" && !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	if !associationExtensionPattern.MatchString(extension) {
		return "", errors.New("请输入有效扩展名，例如 ts 或 html")
	}
	return extension, nil
}

func normalizeAssociationCommand(value string) (string, error) {
	command := strings.TrimSpace(value)
	if command == "" {
		return "", errors.New("打开命令不能为空")
	}
	if strings.ContainsAny(command, "\r\n") {
		return "", errors.New("打开命令只能占一行")
	}
	return command, nil
}

func associationProgID(extension string) string {
	return "WinAdmin.File." + strings.TrimPrefix(extension, ".")
}

func associationRegistryPath(extension string) string {
	return `HKCU\Software\Classes\` + associationProgID(extension)
}

func associationLauncherCommand(extension string) (string, error) {
	launcher, err := associationLauncherPath()
	if err != nil {
		return "", err
	}
	return `"` + launcher + `" ` + extension + ` "%1"`, nil
}

func associationLauncherPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取应用路径失败: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), associationLauncherExecutable), nil
}

func associationCommandArgs(command, filePath string) (string, []string, error) {
	args, err := associationCommandParts(command, filePath)
	if err != nil {
		return "", nil, err
	}
	executable, err := resolveAssociationExecutable(args[0])
	if err != nil {
		return "", nil, err
	}
	return executable, args[1:], nil
}

func associationCommandParts(command, filePath string) ([]string, error) {
	args, err := windows.DecomposeCommandLine(command)
	if err != nil || len(args) == 0 || args[0] == "" {
		return nil, errors.New("打开命令格式无效")
	}
	hasFilePath := false
	for index := 1; index < len(args); index++ {
		if strings.Contains(args[index], "%1") {
			hasFilePath = true
		}
		args[index] = strings.ReplaceAll(args[index], "%1", filePath)
	}
	if !hasFilePath {
		args = append(args, filePath)
	}
	return args, nil
}

func resolveAssociationExecutable(value string) (string, error) {
	if filepath.IsAbs(value) {
		if info, err := os.Stat(value); err == nil && !info.IsDir() {
			return value, nil
		}
	} else if !strings.ContainsAny(value, `\/`) {
		if resolved, err := exec.LookPath(value); err == nil {
			return filepath.Abs(resolved)
		}
		name := value
		if filepath.Ext(name) == "" {
			name += ".exe"
		}
		for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
			key, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\App Paths\`+name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			resolved, _, readErr := key.GetStringValue("")
			key.Close()
			resolved = strings.Trim(resolved, `"`)
			if readErr == nil {
				if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
					return resolved, nil
				}
			}
		}
	}
	return "", fmt.Errorf("找不到程序 %q，请检查名称或填写完整路径", value)
}

func (s *FileAssociationService) List() ([]FileAssociation, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, associationMappings, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return []FileAssociation{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取文件打开方式失败: %w", err)
	}
	defer key.Close()
	names, err := key.ReadValueNames(-1)
	if err != nil {
		return nil, fmt.Errorf("读取扩展名失败: %w", err)
	}
	items := make([]FileAssociation, 0, len(names))
	for _, name := range names {
		extension, err := normalizeAssociationExtension(name)
		if err != nil {
			continue
		}
		progID := associationProgID(extension)
		registered, _, err := key.GetStringValue(extension)
		if err != nil || registered != progID {
			continue
		}
		storedKey, err := registry.OpenKey(registry.CURRENT_USER, associationClasses+progID, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		userCommand, _, readErr := storedKey.GetStringValue(associationCommandValue)
		storedKey.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取打开命令失败: %w", readErr)
		}
		items = append(items, FileAssociation{
			Extension:    extension,
			Command:      userCommand,
			Active:       associationIsActive(extension, progID),
			RegistryPath: associationRegistryPath(extension),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Extension < items[j].Extension })
	return items, nil
}

func (s *FileAssociationService) Save(extension, command string) (FileAssociation, error) {
	extension, err := normalizeAssociationExtension(extension)
	if err != nil {
		return FileAssociation{}, err
	}
	command, err = normalizeAssociationCommand(command)
	if err != nil {
		return FileAssociation{}, err
	}
	executable, _, err := associationCommandArgs(command, `C:\sample.ts`)
	if err != nil {
		return FileAssociation{}, err
	}
	launcher, err := associationLauncherCommand(extension)
	if err != nil {
		return FileAssociation{}, err
	}
	// 注册为可选打开方式，当前用户的默认选择由 Windows 设置维护。
	progID := associationProgID(extension)
	if err := setAssociationValue(associationClasses+progID, "", associationAppName+" "+strings.TrimPrefix(extension, ".")); err != nil {
		return FileAssociation{}, err
	}
	if err := setAssociationValue(associationClasses+progID, associationCommandValue, command); err != nil {
		return FileAssociation{}, err
	}
	if err := setAssociationValue(associationClasses+progID, associationExecutableValue, executable); err != nil {
		return FileAssociation{}, err
	}
	if err := setAssociationValue(associationClasses+progID+`\shell\open\command`, "", launcher); err != nil {
		return FileAssociation{}, err
	}
	if err := setAssociationValue(associationClasses+extension+`\OpenWithProgids`, progID, ""); err != nil {
		return FileAssociation{}, err
	}
	if err := setAssociationValue(associationCapabilities, "ApplicationName", associationAppName); err != nil {
		return FileAssociation{}, err
	}
	if err := setAssociationValue(associationCapabilities, "ApplicationDescription", "按扩展名运行自定义打开命令"); err != nil {
		return FileAssociation{}, err
	}
	if err := setAssociationValue(associationMappings, extension, progID); err != nil {
		return FileAssociation{}, err
	}
	if err := setAssociationValue(`Software\RegisteredApplications`, associationAppName, associationCapabilities); err != nil {
		return FileAssociation{}, err
	}
	notifyShellAssociationChanged()
	return FileAssociation{
		Extension:    extension,
		Command:      command,
		Active:       associationIsActive(extension, progID),
		RegistryPath: associationRegistryPath(extension),
	}, nil
}

func (s *FileAssociationService) Rename(previousExtension, extension, command string) (FileAssociation, error) {
	previousExtension, err := normalizeAssociationExtension(previousExtension)
	if err != nil {
		return FileAssociation{}, err
	}
	extension, err = normalizeAssociationExtension(extension)
	if err != nil {
		return FileAssociation{}, err
	}
	if _, err := normalizeAssociationCommand(command); err != nil {
		return FileAssociation{}, err
	}
	registered, err := associationIsRegistered(previousExtension)
	if err != nil {
		return FileAssociation{}, err
	}
	if !registered {
		return FileAssociation{}, errors.New("原扩展名的打开方式不存在")
	}
	if previousExtension == extension {
		return s.Save(extension, command)
	}
	registered, err = associationIsRegistered(extension)
	if err != nil {
		return FileAssociation{}, err
	}
	if registered {
		return FileAssociation{}, errors.New("目标扩展名已有打开方式")
	}
	item, err := s.Save(extension, command)
	if err != nil {
		return FileAssociation{}, err
	}
	if err := s.Delete(previousExtension); err != nil {
		if rollbackErr := s.Delete(extension); rollbackErr != nil {
			return FileAssociation{}, fmt.Errorf("清理原扩展名失败: %w；回退新扩展名失败: %v", err, rollbackErr)
		}
		return FileAssociation{}, fmt.Errorf("清理原扩展名失败: %w", err)
	}
	return item, nil
}

func associationIsRegistered(extension string) (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, associationMappings, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取文件打开方式失败: %w", err)
	}
	defer key.Close()
	progID, _, err := key.GetStringValue(extension)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取扩展名失败: %w", err)
	}
	return progID == associationProgID(extension), nil
}

func (s *FileAssociationService) Delete(extension string) error {
	extension, err := normalizeAssociationExtension(extension)
	if err != nil {
		return err
	}
	progID := associationProgID(extension)
	key, err := registry.OpenKey(registry.CURRENT_USER, associationMappings, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return errors.New("未找到该扩展名的打开命令")
	}
	if err != nil {
		return fmt.Errorf("读取文件打开方式失败: %w", err)
	}
	keyOpen := true
	defer func() {
		if keyOpen {
			key.Close()
		}
	}()
	registered, _, err := key.GetStringValue(extension)
	if err != nil || registered != progID {
		return errors.New("未找到该扩展名的打开命令")
	}
	// 注销 ProgID 后，Windows 会忽略指向它的旧默认选择，无需修改 UserChoice。
	if err := key.DeleteValue(extension); err != nil {
		return fmt.Errorf("删除扩展名注册失败: %w", err)
	}
	remaining, err := key.ReadValueNames(-1)
	if err != nil {
		return fmt.Errorf("检查剩余扩展名失败: %w", err)
	}
	openWith, err := registry.OpenKey(registry.CURRENT_USER, associationClasses+extension+`\OpenWithProgids`, registry.SET_VALUE)
	if err == nil {
		_ = openWith.DeleteValue(progID)
		openWith.Close()
	}
	deleteEmptyAssociationKey(associationClasses + extension + `\OpenWithProgids`)
	deleteEmptyAssociationKey(associationClasses + extension)
	for _, suffix := range []string{`\shell\open\command`, `\shell\open`, `\shell`, ""} {
		if err := registry.DeleteKey(registry.CURRENT_USER, associationClasses+progID+suffix); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("删除打开命令失败: %w", err)
		}
	}
	if len(remaining) == 0 {
		key.Close()
		keyOpen = false
		registeredApps, openErr := registry.OpenKey(registry.CURRENT_USER, `Software\RegisteredApplications`, registry.SET_VALUE)
		if openErr == nil {
			_ = registeredApps.DeleteValue(associationAppName)
			registeredApps.Close()
		}
		for _, path := range []string{associationMappings, associationCapabilities, `Software\win-admin`} {
			_ = registry.DeleteKey(registry.CURRENT_USER, path)
		}
	}
	notifyShellAssociationChanged()
	return nil
}

func (s *FileAssociationService) OpenDefaultApps() error {
	uri, err := windows.UTF16PtrFromString("ms-settings:defaultapps?registeredAppUser=" + url.PathEscape(associationAppName))
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, nil, uri, nil, nil, 1); err != nil {
		return fmt.Errorf("打开 Windows 默认应用设置失败: %w", err)
	}
	return nil
}

func setAssociationValue(path, name, value string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("注册打开方式失败: %w", err)
	}
	defer key.Close()
	if err := key.SetStringValue(name, value); err != nil {
		return fmt.Errorf("写入打开方式失败: %w", err)
	}
	return nil
}

func deleteEmptyAssociationKey(path string) {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return
	}
	values, valueErr := key.ReadValueNames(-1)
	children, childErr := key.ReadSubKeyNames(-1)
	key.Close()
	if valueErr == nil && childErr == nil && len(values) == 0 && len(children) == 0 {
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
	}
}

func associationIsActive(extension, progID string) bool {
	choice, err := registry.OpenKey(registry.CURRENT_USER, associationUserChoice+extension+`\UserChoice`, registry.QUERY_VALUE)
	if err == nil {
		selected, _, readErr := choice.GetStringValue("ProgId")
		choice.Close()
		return readErr == nil && strings.EqualFold(selected, progID)
	}
	extensionKey, err := registry.OpenKey(registry.CLASSES_ROOT, extension, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer extensionKey.Close()
	selected, _, err := extensionKey.GetStringValue("")
	return err == nil && strings.EqualFold(selected, progID)
}
