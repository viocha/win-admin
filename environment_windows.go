//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	systemEnvironmentPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	userEnvironmentPath   = `Environment`
	wmSettingChange       = 0x001A
	hwndBroadcast         = 0xffff
	smtoAbortIfHung       = 0x0002
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
)

func (s *EnvironmentVariableService) List(scope string) ([]EnvironmentVariable, error) {
	root, path, normalized, err := environmentRegistryLocation(scope)
	if err != nil {
		return nil, err
	}
	items, err := readEnvironmentVariables(root, path, normalized)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func (s *EnvironmentVariableService) Save(request SaveEnvironmentVariableRequest) error {
	variable := request.Variable
	variable.Name = strings.TrimSpace(variable.Name)
	if variable.Name == "" {
		return errors.New("变量名称不能为空")
	}
	if strings.ContainsRune(variable.Name, '=') {
		return errors.New("变量名称不能包含等号")
	}

	root, path, _, err := environmentRegistryLocation(variable.Scope)
	if err != nil {
		return err
	}
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开环境变量注册表失败: %w", err)
	}
	defer key.Close()

	previousName := strings.TrimSpace(request.PreviousName)
	if previousName == "" || !strings.EqualFold(previousName, variable.Name) {
		if _, _, existingErr := key.GetStringValue(variable.Name); existingErr == nil {
			return errors.New("环境变量已存在")
		} else if !errors.Is(existingErr, registry.ErrNotExist) {
			return fmt.Errorf("检查环境变量失败: %w", existingErr)
		}
	}

	expandable := variable.Expandable || strings.Contains(variable.Value, "%")
	if expandable {
		err = key.SetExpandStringValue(variable.Name, variable.Value)
	} else {
		err = key.SetStringValue(variable.Name, variable.Value)
	}
	if err != nil {
		return fmt.Errorf("保存环境变量失败: %w", err)
	}

	if previousName != "" && !strings.EqualFold(previousName, variable.Name) {
		if deleteErr := key.DeleteValue(previousName); deleteErr != nil && !errors.Is(deleteErr, registry.ErrNotExist) {
			return fmt.Errorf("删除旧环境变量失败: %w", deleteErr)
		}
	}
	broadcastEnvironmentChange()
	return nil
}

func (s *EnvironmentVariableService) Delete(scope string, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("变量名称不能为空")
	}
	root, path, _, err := environmentRegistryLocation(scope)
	if err != nil {
		return err
	}
	key, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开环境变量注册表失败: %w", err)
	}
	defer key.Close()
	if err := key.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("删除环境变量失败: %w", err)
	}
	broadcastEnvironmentChange()
	return nil
}

func (s *EnvironmentVariableService) GetPath(scope string) ([]EnvironmentPathEntry, error) {
	root, path, normalized, err := environmentRegistryLocation(scope)
	if err != nil {
		return nil, err
	}
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("打开环境变量注册表失败: %w", err)
	}
	defer key.Close()
	valueName := environmentPathValueName(key)
	value, _, err := key.GetStringValue(valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return []EnvironmentPathEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 PATH 失败: %w", err)
	}
	return analyzePathEntries(normalized, splitPathEntries(value))
}

func (s *EnvironmentVariableService) AnalyzePath(scope string, entries []string) ([]EnvironmentPathEntry, error) {
	_, _, normalized, err := environmentRegistryLocation(scope)
	if err != nil {
		return nil, err
	}
	return analyzePathEntries(normalized, entries)
}

func (s *EnvironmentVariableService) SavePath(request SaveEnvironmentPathRequest) error {
	root, path, _, err := environmentRegistryLocation(request.Scope)
	if err != nil {
		return err
	}
	entries := make([]string, 0, len(request.Entries))
	for _, entry := range request.Entries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			entries = append(entries, entry)
		}
	}
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开环境变量注册表失败: %w", err)
	}
	defer key.Close()
	valueName := environmentPathValueName(key)
	if err := key.SetExpandStringValue(valueName, strings.Join(entries, ";")); err != nil {
		return fmt.Errorf("保存 PATH 失败: %w", err)
	}
	broadcastEnvironmentChange()
	return nil
}

func environmentPathValueName(key registry.Key) string {
	names, err := key.ReadValueNames(-1)
	if err == nil {
		for _, name := range names {
			if strings.EqualFold(name, "Path") {
				return name
			}
		}
	}
	return "Path"
}

func environmentRegistryLocation(scope string) (registry.Key, string, string, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "system":
		return registry.LOCAL_MACHINE, systemEnvironmentPath, "system", nil
	case "user":
		return registry.CURRENT_USER, userEnvironmentPath, "user", nil
	default:
		return 0, "", "", errors.New("环境变量作用域无效")
	}
}

func readEnvironmentVariables(root registry.Key, path string, scope string) ([]EnvironmentVariable, error) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("打开环境变量注册表失败: %w", err)
	}
	defer key.Close()
	names, err := key.ReadValueNames(-1)
	if err != nil {
		return nil, fmt.Errorf("读取环境变量名称失败: %w", err)
	}
	items := make([]EnvironmentVariable, 0, len(names))
	for _, name := range names {
		value, valueType, valueErr := key.GetStringValue(name)
		if valueErr != nil {
			continue
		}
		items = append(items, EnvironmentVariable{
			Name:       name,
			Value:      value,
			Scope:      scope,
			Expandable: valueType == registry.EXPAND_SZ,
		})
	}
	return items, nil
}

func splitPathEntries(value string) []string {
	parts := strings.Split(value, ";")
	entries := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			entries = append(entries, part)
		}
	}
	return entries
}

func analyzePathEntries(scope string, entries []string) ([]EnvironmentPathEntry, error) {
	environment, err := currentRegistryEnvironment()
	if err != nil {
		return nil, err
	}
	result := make([]EnvironmentPathEntry, 0, len(entries))
	for _, value := range entries {
		value = strings.TrimSpace(value)
		expanded := expandRegistryEnvironment(value, environment)
		checkPath := strings.Trim(strings.TrimSpace(expanded), `"`)
		_, statErr := os.Stat(checkPath)
		result = append(result, EnvironmentPathEntry{
			Value:    value,
			Expanded: expanded,
			Exists:   checkPath != "" && statErr == nil,
		})
	}
	return result, nil
}

func currentRegistryEnvironment() (map[string]string, error) {
	values := make(map[string]string, 128)
	for _, item := range os.Environ() {
		if index := strings.IndexByte(item, '='); index > 0 {
			values[strings.ToUpper(item[:index])] = item[index+1:]
		}
	}
	for _, source := range []struct {
		root  registry.Key
		path  string
		scope string
	}{
		{registry.LOCAL_MACHINE, systemEnvironmentPath, "system"},
		{registry.CURRENT_USER, userEnvironmentPath, "user"},
	} {
		items, err := readEnvironmentVariables(source.root, source.path, source.scope)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			values[strings.ToUpper(item.Name)] = item.Value
		}
	}
	return values, nil
}

func expandRegistryEnvironment(value string, environment map[string]string) string {
	result := value
	for range 8 {
		var builder strings.Builder
		changed := false
		for index := 0; index < len(result); {
			start := strings.IndexByte(result[index:], '%')
			if start < 0 {
				builder.WriteString(result[index:])
				break
			}
			start += index
			end := strings.IndexByte(result[start+1:], '%')
			if end < 0 {
				builder.WriteString(result[index:])
				break
			}
			end += start + 1
			builder.WriteString(result[index:start])
			name := result[start+1 : end]
			if replacement, ok := environment[strings.ToUpper(name)]; ok {
				builder.WriteString(replacement)
				changed = true
			} else {
				builder.WriteString(result[start : end+1])
			}
			index = end + 1
		}
		next := builder.String()
		if !changed || next == result {
			return result
		}
		result = next
	}
	return result
}

func broadcastEnvironmentChange() {
	name, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	_, _, _ = procSendMessageTimeoutW.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(name)),
		smtoAbortIfHung,
		5000,
		uintptr(unsafe.Pointer(&result)),
	)
}
