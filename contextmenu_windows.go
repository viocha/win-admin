//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const classesRoot = `Software\Classes`

var safeVerbPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type menuRegistrySource struct {
	root     registry.Key
	hive     string
	path     string
	kind     string
	readOnly bool
}

func (s *ContextMenuService) Status() ContextMenuStatus {
	return ContextMenuStatus{
		Elevated:     windows.GetCurrentProcessToken().IsElevated(),
		RegistryRoot: `HKLM/HKCU\Software\Classes`,
		Scope:        "machine+user",
	}
}

func (s *ContextMenuService) List(target string) ([]MenuEntry, error) {
	sources, err := menuSourcesForTarget(target)
	if err != nil {
		return nil, err
	}
	entries := make([]MenuEntry, 0, 64)
	for _, source := range sources {
		var sourceEntries []MenuEntry
		if source.kind == "handler" {
			sourceEntries, err = readContextMenuHandlers(source, target)
		} else {
			sourceEntries, err = readStaticMenuEntries(source, target)
		}
		if err == nil {
			entries = append(entries, sourceEntries...)
		}
	}
	entries = dedupeMergedClassEntries(entries)

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].ReadOnly != entries[j].ReadOnly {
			return !entries[i].ReadOnly
		}
		lowValueI := entries[i].Kind != "submenu" && strings.TrimSpace(entries[i].Command) == ""
		lowValueJ := entries[j].Kind != "submenu" && strings.TrimSpace(entries[j].Command) == ""
		if lowValueI != lowValueJ {
			return !lowValueI
		}
		return strings.ToLower(entries[i].Label) < strings.ToLower(entries[j].Label)
	})
	return entries, nil
}

func dedupeMergedClassEntries(entries []MenuEntry) []MenuEntry {
	result := make([]MenuEntry, 0, len(entries))
	indexByPath := make(map[string]int, len(entries))
	for _, entry := range entries {
		path := strings.ToLower(entry.RegistryPath)
		logicalPath := strings.TrimPrefix(strings.TrimPrefix(path, `hklm\`), `hkcu\`)
		if index, exists := indexByPath[logicalPath]; exists {
			if strings.HasPrefix(path, `hkcu\`) {
				result[index] = entry
			}
			continue
		}
		indexByPath[logicalPath] = len(result)
		result = append(result, entry)
	}
	return result
}

func menuSourcesForTarget(target string) ([]menuRegistrySource, error) {
	primary, err := targetShellPath(target)
	if err != nil {
		return nil, err
	}
	sources := []menuRegistrySource{{
		root: registry.LOCAL_MACHINE, hive: "HKLM", path: primary, kind: "static",
	}}
	addPair := func(path, kind string) {
		sources = append(sources,
			menuRegistrySource{root: registry.LOCAL_MACHINE, hive: "HKLM", path: path, kind: kind, readOnly: true},
			menuRegistrySource{root: registry.CURRENT_USER, hive: "HKCU", path: path, kind: kind, readOnly: true},
		)
	}
	sources = append(sources, menuRegistrySource{
		root: registry.CURRENT_USER, hive: "HKCU", path: primary, kind: "static", readOnly: true,
	})
	switch target {
	case "file":
		addPair(classesRoot+`\*\shellex\ContextMenuHandlers`, "handler")
		addPair(classesRoot+`\AllFileSystemObjects\shell`, "static")
		addPair(classesRoot+`\AllFileSystemObjects\shellex\ContextMenuHandlers`, "handler")
	case "directory":
		addPair(classesRoot+`\Directory\shellex\ContextMenuHandlers`, "handler")
		addPair(classesRoot+`\Folder\shell`, "static")
		addPair(classesRoot+`\Folder\shellex\ContextMenuHandlers`, "handler")
		addPair(classesRoot+`\AllFileSystemObjects\shell`, "static")
		addPair(classesRoot+`\AllFileSystemObjects\shellex\ContextMenuHandlers`, "handler")
	case "background":
		addPair(classesRoot+`\Directory\Background\shellex\ContextMenuHandlers`, "handler")
		addPair(classesRoot+`\DesktopBackground\shell`, "static")
		addPair(classesRoot+`\DesktopBackground\shellex\ContextMenuHandlers`, "handler")
	}
	return sources, nil
}

func readStaticMenuEntries(source menuRegistrySource, target string) ([]MenuEntry, error) {
	key, err := registry.OpenKey(source.root, source.path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return []MenuEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer key.Close()
	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	entries := make([]MenuEntry, 0, len(names))
	for _, name := range names {
		entry, readErr := readMenuEntryAt(source.root, source.hive, source.path, name, target, 0, source.readOnly)
		if readErr != nil {
			continue
		}
		if source.readOnly {
			entry.ID = readOnlyMenuID(entry.RegistryPath)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func readContextMenuHandlers(source menuRegistrySource, target string) ([]MenuEntry, error) {
	key, err := registry.OpenKey(source.root, source.path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return []MenuEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer key.Close()
	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	entries := make([]MenuEntry, 0, len(names))
	for _, name := range names {
		path := source.path + `\` + name
		handlerKey, openErr := registry.OpenKey(source.root, path, registry.QUERY_VALUE)
		if openErr != nil {
			continue
		}
		value := strings.TrimSpace(readString(handlerKey, ""))
		enabled := !hasValue(handlerKey, "LegacyDisable")
		handlerKey.Close()
		clsid := value
		if !looksLikeCLSID(clsid) && looksLikeCLSID(name) {
			clsid = name
		}
		if strings.HasPrefix(clsid, "-") {
			enabled = false
			clsid = strings.TrimPrefix(clsid, "-")
		}
		friendly, server := resolveShellExtension(clsid)
		label := strings.TrimSpace(name)
		if friendly != "" && (looksLikeCLSID(label) || strings.EqualFold(label, clsid)) {
			label = friendly
		}
		if label == "" {
			label = friendly
		}
		if label == "" {
			label = clsid
		}
		if shellExtensionBlocked(clsid) {
			enabled = false
		}
		registryPath := source.hive + `\` + path
		entries = append(entries, MenuEntry{
			ID:              readOnlyMenuID(registryPath),
			Label:           label,
			Target:          target,
			Kind:            "command",
			Command:         server,
			CommandTemplate: server,
			Icon:            server,
			Position:        "normal",
			Enabled:         enabled,
			ReadOnly:        true,
			CanEdit:         false,
			CanEnable:       looksLikeCLSID(clsid),
			CanPosition:     false,
			CanShift:        false,
			CanDelete:       true,
			RegistryPath:    registryPath,
			Children:        []MenuEntry{},
		})
	}
	return entries, nil
}

func resolveShellExtension(clsid string) (string, string) {
	if !looksLikeCLSID(clsid) {
		return "", ""
	}
	key, err := registry.OpenKey(registry.CLASSES_ROOT, `CLSID\`+clsid, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	friendly := strings.TrimSpace(readString(key, ""))
	key.Close()
	for _, child := range []string{"InprocServer32", "LocalServer32"} {
		serverKey, openErr := registry.OpenKey(registry.CLASSES_ROOT, `CLSID\`+clsid+`\`+child, registry.QUERY_VALUE)
		if openErr != nil {
			continue
		}
		server := strings.TrimSpace(readString(serverKey, ""))
		serverKey.Close()
		if server != "" {
			return friendly, server
		}
	}
	return friendly, ""
}

func shellExtensionBlocked(clsid string) bool {
	if !looksLikeCLSID(clsid) {
		return false
	}
	const blockedPath = `Software\Microsoft\Windows\CurrentVersion\Shell Extensions\Blocked`
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		key, err := registry.OpenKey(root, blockedPath, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		blocked := hasValue(key, clsid)
		key.Close()
		if blocked {
			return true
		}
	}
	return false
}

func looksLikeCLSID(value string) bool {
	value = strings.TrimSpace(strings.TrimPrefix(value, "-"))
	return len(value) == 38 && strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}")
}

func readOnlyMenuID(registryPath string) string {
	return "readonly:" + strings.ToLower(registryPath)
}

func (s *ContextMenuService) Save(request SaveMenuRequest) (MenuEntry, error) {
	entry := request.Entry
	if strings.TrimSpace(entry.Label) == "" {
		return MenuEntry{}, errors.New("菜单名称不能为空")
	}
	if entry.Kind != "command" && entry.Kind != "submenu" {
		return MenuEntry{}, errors.New("不支持的菜单类型")
	}
	if entry.Target == "" {
		entry.Target = "file"
	}
	if _, err := targetShellPath(entry.Target); err != nil {
		return MenuEntry{}, err
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return MenuEntry{}, errors.New("当前进程未以管理员权限运行，无法写入 HKLM")
	}

	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = newVerbID(entry.Label)
	}
	if !validVerbID(entry.ID) {
		return MenuEntry{}, errors.New("无效的菜单 ID")
	}
	entry.Enabled = request.Entry.Enabled
	entry.Managed = true
	entry.ReadOnly = false

	entry = normalizeMenuCommandPaths(entry)

	base, _ := targetShellPath(entry.Target)
	fullPath := base + `\` + entry.ID
	if err := deleteTreeIfExists(registry.LOCAL_MACHINE, fullPath); err != nil {
		return MenuEntry{}, fmt.Errorf("清理旧菜单失败: %w", err)
	}
	if err := writeMenuEntry(base, entry); err != nil {
		return MenuEntry{}, err
	}

	if request.PreviousID != "" && (request.PreviousID != entry.ID || request.PreviousTarget != entry.Target) {
		oldBase, err := targetShellPath(request.PreviousTarget)
		if err == nil && validVerbID(request.PreviousID) {
			_ = deleteTreeIfExists(registry.LOCAL_MACHINE, oldBase+`\`+request.PreviousID)
		}
	}

	notifyShellAssociationChanged()
	return readMenuEntry(base, entry.ID, entry.Target, 0)
}

func (s *ContextMenuService) Delete(registryPath string) error {
	root, _, path, err := parseMenuRegistryPath(registryPath)
	if err != nil {
		return err
	}
	if root == registry.LOCAL_MACHINE && !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("当前进程未以管理员权限运行，无法写入 HKLM")
	}
	if err := deleteTreeIfExists(root, path); err != nil {
		return fmt.Errorf("删除菜单失败: %w", err)
	}
	notifyShellAssociationChanged()
	return nil
}

func (s *ContextMenuService) SetEnabled(registryPath string, enabled bool) error {
	root, _, path, err := parseMenuRegistryPath(registryPath)
	if err != nil {
		return err
	}
	if root == registry.LOCAL_MACHINE && !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("当前进程未以管理员权限运行，无法写入 HKLM")
	}
	if isContextMenuHandlerPath(path) {
		clsid, clsidErr := contextMenuHandlerCLSID(root, path)
		if clsidErr != nil {
			return clsidErr
		}
		if err := setShellExtensionBlocked(clsid, !enabled); err != nil {
			return err
		}
		notifyShellAssociationChanged()
		return nil
	}

	key, err := registry.OpenKey(root, path, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("打开菜单注册表失败: %w", err)
	}
	defer key.Close()

	if enabled {
		_ = key.DeleteValue("LegacyDisable")
	} else if err := key.SetStringValue("LegacyDisable", ""); err != nil {
		return fmt.Errorf("禁用菜单失败: %w", err)
	}
	notifyShellAssociationChanged()
	return nil
}

func (s *ContextMenuService) SetPosition(registryPath, position string) error {
	root, _, path, err := parseMenuRegistryPath(registryPath)
	if err != nil {
		return err
	}
	if isContextMenuHandlerPath(path) {
		return errors.New("Shell 扩展处理器不支持菜单位置")
	}
	if root == registry.LOCAL_MACHINE && !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("当前进程未以管理员权限运行，无法写入 HKLM")
	}
	key, err := registry.OpenKey(root, path, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("打开菜单注册表失败: %w", err)
	}
	defer key.Close()
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "top":
		err = key.SetStringValue("Position", "Top")
	case "bottom":
		err = key.SetStringValue("Position", "Bottom")
	case "normal":
		err = key.DeleteValue("Position")
		if errors.Is(err, registry.ErrNotExist) {
			err = nil
		}
	default:
		return errors.New("无效的菜单位置")
	}
	if err != nil {
		return fmt.Errorf("修改菜单位置失败: %w", err)
	}
	notifyShellAssociationChanged()
	return nil
}

func (s *ContextMenuService) SetShiftOnly(registryPath string, shiftOnly bool) error {
	root, _, path, err := parseMenuRegistryPath(registryPath)
	if err != nil {
		return err
	}
	if isContextMenuHandlerPath(path) {
		return errors.New("Shell 扩展处理器不支持 Shift-only")
	}
	if root == registry.LOCAL_MACHINE && !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("当前进程未以管理员权限运行，无法写入 HKLM")
	}
	key, err := registry.OpenKey(root, path, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("打开菜单注册表失败: %w", err)
	}
	defer key.Close()
	if shiftOnly {
		err = key.SetStringValue("Extended", "")
	} else {
		err = key.DeleteValue("Extended")
		if errors.Is(err, registry.ErrNotExist) {
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("修改 Shift 显示设置失败: %w", err)
	}
	notifyShellAssociationChanged()
	return nil
}

func (s *ContextMenuService) InferIcon(command string) string {
	candidate := firstCommandToken(command)
	if candidate == "" {
		return ""
	}
	candidate = os.ExpandEnv(candidate)
	if abs, err := exec.LookPath(candidate); err == nil {
		candidate = abs
	}
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		if abs, absErr := filepath.Abs(candidate); absErr == nil {
			return abs
		}
		return candidate
	}
	return ""
}

// ResolveCommand 将命令行首个裸命令名解析为绝对可执行文件路径。
// Explorer 上下文菜单不会可靠继承当前用户 PATH，因此保存前需要固化可执行文件位置。
func (s *ContextMenuService) ResolveCommand(command string) string {
	return resolveCommandExecutable(command)
}

func (s *ContextMenuService) PickIconFile() (string, error) {
	result, err := application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false).
		SetTitle("选择图标来源").
		AddFilter("图标资源", "*.exe;*.dll;*.ico").
		PromptForSingleSelection()
	if err != nil {
		if isDialogCancelled(err) {
			return "", nil
		}
		return "", err
	}
	return result, nil
}

func targetShellPath(target string) (string, error) {
	switch target {
	case "file":
		return classesRoot + `\*\shell`, nil
	case "directory":
		return classesRoot + `\Directory\shell`, nil
	case "background":
		return classesRoot + `\Directory\Background\shell`, nil
	default:
		return "", fmt.Errorf("不支持的目标类型: %s", target)
	}
}

func readMenuEntry(parentPath, id, target string, depth int) (MenuEntry, error) {
	return readMenuEntryAt(registry.LOCAL_MACHINE, "HKLM", parentPath, id, target, depth, false)
}

func readMenuEntryAt(root registry.Key, hive, parentPath, id, target string, depth int, forceReadOnly bool) (MenuEntry, error) {
	if depth > 12 {
		return MenuEntry{}, errors.New("菜单层级过深")
	}
	path := parentPath + `\` + id
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return MenuEntry{}, err
	}
	defer key.Close()

	label := readString(key, "MUIVerb")
	if label == "" {
		label = readString(key, "")
	}
	if label == "" {
		label = id
	}

	entry := MenuEntry{
		ID:           id,
		Label:        label,
		Target:       target,
		Icon:         readString(key, "Icon"),
		Position:     strings.ToLower(readString(key, "Position")),
		ShiftOnly:    hasValue(key, "Extended"),
		Enabled:      !hasValue(key, "LegacyDisable"),
		Managed:      readString(key, "win-admin-managed") == "1",
		ReadOnly:     forceReadOnly,
		CanEdit:      !forceReadOnly,
		CanEnable:    true,
		CanPosition:  true,
		CanShift:     true,
		CanDelete:    true,
		RegistryPath: hive + `\` + path,
		Children:     []MenuEntry{},
	}
	if entry.Position != "top" && entry.Position != "bottom" {
		entry.Position = "normal"
	}

	entry.Command = readCommandAt(root, path)
	entry.CommandTemplate = entry.Command

	children, _ := readChildrenAt(root, hive, path+`\shell`, target, depth+1, forceReadOnly)
	entry.Children = children
	if len(children) > 0 {
		entry.Kind = "submenu"
		return entry, nil
	}

	if entry.Command != "" {
		entry.Kind = "command"
		return entry, nil
	}

	if hasValue(key, "SubCommands") || hasValue(key, "ExplorerCommandHandler") {
		entry.Kind = "submenu"
		entry.ReadOnly = true
		entry.CanEdit = false
		return entry, nil
	}

	entry.Kind = "command"
	entry.ReadOnly = true
	entry.CanEdit = false
	return entry, nil
}

func readChildren(shellPath, target string, depth int) ([]MenuEntry, error) {
	return readChildrenAt(registry.LOCAL_MACHINE, "HKLM", shellPath, target, depth, false)
}

func readChildrenAt(root registry.Key, hive, shellPath, target string, depth int, forceReadOnly bool) ([]MenuEntry, error) {
	key, err := registry.OpenKey(root, shellPath, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return []MenuEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer key.Close()

	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	children := make([]MenuEntry, 0, len(names))
	for _, name := range names {
		child, readErr := readMenuEntryAt(root, hive, shellPath, name, target, depth, forceReadOnly)
		if readErr == nil {
			children = append(children, child)
		}
	}
	return children, nil
}

func readCommand(path string) string {
	return readCommandAt(registry.LOCAL_MACHINE, path)
}

func readCommandAt(root registry.Key, path string) string {
	key, err := registry.OpenKey(root, path+`\command`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	return readString(key, "")
}

func readString(key registry.Key, name string) string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

func hasValue(key registry.Key, name string) bool {
	names, err := key.ReadValueNames(-1)
	if err != nil {
		return false
	}
	for _, current := range names {
		if strings.EqualFold(current, name) {
			return true
		}
	}
	return false
}

func writeMenuEntry(parentPath string, entry MenuEntry) error {
	if entry.ID == "" {
		entry.ID = newVerbID(entry.Label)
	}
	if !validVerbID(entry.ID) {
		return errors.New("无效的菜单 ID")
	}
	path := parentPath + `\` + entry.ID
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, path, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("创建菜单注册表失败: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue("MUIVerb", entry.Label); err != nil {
		return err
	}
	_ = key.SetStringValue("win-admin-managed", "1")
	if entry.Icon != "" {
		_ = key.SetStringValue("Icon", entry.Icon)
	}
	switch strings.ToLower(entry.Position) {
	case "top":
		_ = key.SetStringValue("Position", "Top")
	case "bottom":
		_ = key.SetStringValue("Position", "Bottom")
	}
	if !entry.Enabled {
		_ = key.SetStringValue("LegacyDisable", "")
	}
	if entry.ShiftOnly {
		_ = key.SetStringValue("Extended", "")
	}

	if entry.Kind == "submenu" {
		_ = key.SetStringValue("SubCommands", "")
		shellPath := path + `\shell`
		for index, child := range entry.Children {
			if strings.TrimSpace(child.Label) == "" {
				continue
			}
			if child.ID == "" {
				child.ID = fmt.Sprintf("%03d.%s", index+1, newVerbID(child.Label))
			}
			child.Target = entry.Target
			child.Managed = true
			if child.Kind == "" {
				child.Kind = "command"
			}
			if err := writeMenuEntry(shellPath, child); err != nil {
				return err
			}
		}
		return nil
	}

	command := strings.TrimSpace(entry.Command)
	if command == "" {
		command = strings.TrimSpace(entry.CommandTemplate)
	}
	if command == "" {
		return fmt.Errorf("菜单 %q 缺少命令", entry.Label)
	}
	commandKey, _, err := registry.CreateKey(registry.LOCAL_MACHINE, path+`\command`, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("创建命令注册表失败: %w", err)
	}
	defer commandKey.Close()
	if err := commandKey.SetStringValue("", command); err != nil {
		return fmt.Errorf("写入命令失败: %w", err)
	}
	return nil
}

func deleteTreeIfExists(root registry.Key, path string) error {
	key, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	names, readErr := key.ReadSubKeyNames(-1)
	key.Close()
	if readErr != nil {
		return readErr
	}
	for _, name := range names {
		if err := deleteTreeIfExists(root, path+`\`+name); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(root, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func parseMenuRegistryPath(registryPath string) (registry.Key, string, string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(registryPath, "/", `\`))
	var root registry.Key
	var hive string
	switch {
	case strings.HasPrefix(strings.ToUpper(value), `HKLM\`):
		root, hive, value = registry.LOCAL_MACHINE, "HKLM", value[5:]
	case strings.HasPrefix(strings.ToUpper(value), `HKCU\`):
		root, hive, value = registry.CURRENT_USER, "HKCU", value[5:]
	default:
		return 0, "", "", errors.New("无效的上下文菜单注册表路径")
	}
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, strings.ToLower(classesRoot)+`\`) ||
		(!strings.Contains(lower, `\shell\`) && !strings.Contains(lower, `\shellex\contextmenuhandlers\`)) {
		return 0, "", "", errors.New("注册表路径不属于受支持的上下文菜单位置")
	}
	return root, hive, value, nil
}

func isContextMenuHandlerPath(path string) bool {
	return strings.Contains(strings.ToLower(path), `\shellex\contextmenuhandlers\`)
}

func contextMenuHandlerCLSID(root registry.Key, path string) (string, error) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("读取 Shell 扩展处理器失败: %w", err)
	}
	defer key.Close()
	value := strings.TrimSpace(strings.TrimPrefix(readString(key, ""), "-"))
	if looksLikeCLSID(value) {
		return value, nil
	}
	name := filepath.Base(path)
	if looksLikeCLSID(name) {
		return name, nil
	}
	return "", errors.New("未读取到 Shell 扩展 CLSID")
}

func setShellExtensionBlocked(clsid string, blocked bool) error {
	if !looksLikeCLSID(clsid) {
		return errors.New("Shell 扩展 CLSID 无效")
	}
	const blockedPath = `Software\Microsoft\Windows\CurrentVersion\Shell Extensions\Blocked`
	if blocked {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, blockedPath, registry.ALL_ACCESS)
		if err != nil {
			return fmt.Errorf("打开 Shell 扩展禁用列表失败: %w", err)
		}
		defer key.Close()
		if err := key.SetStringValue(clsid, ""); err != nil {
			return fmt.Errorf("禁用 Shell 扩展失败: %w", err)
		}
		return nil
	}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		key, err := registry.OpenKey(root, blockedPath, registry.ALL_ACCESS)
		if err != nil {
			continue
		}
		deleteErr := key.DeleteValue(clsid)
		key.Close()
		if deleteErr != nil && !errors.Is(deleteErr, registry.ErrNotExist) {
			return fmt.Errorf("启用 Shell 扩展失败: %w", deleteErr)
		}
	}
	return nil
}

func validVerbID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, `\/`+"\x00")
}

func sanitizeVerbID(value string) string {
	value = strings.TrimSpace(value)
	value = safeVerbPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-_")
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}

func newVerbID(label string) string {
	base := sanitizeVerbID(label)
	if base == "" {
		base = "menu"
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "win-admin." + base
	}
	return "win-admin." + base + "." + hex.EncodeToString(buf)
}

func firstCommandToken(command string) string {
	token, _ := splitCommandExecutable(command)
	return token
}

func splitCommandExecutable(command string) (string, string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", ""
	}
	if strings.HasPrefix(command, `"`) {
		if end := strings.Index(command[1:], `"`); end >= 0 {
			closingQuote := end + 1
			return strings.TrimSpace(command[1:closingQuote]), command[closingQuote+1:]
		}
	}
	if end := strings.IndexAny(command, " \t\r\n"); end >= 0 {
		return strings.Trim(command[:end], `"`), command[end:]
	}
	return strings.Trim(command, `"`), ""
}

func normalizeMenuCommandPaths(entry MenuEntry) MenuEntry {
	if entry.Kind == "command" || (entry.Kind == "" && len(entry.Children) == 0) {
		command := strings.TrimSpace(entry.Command)
		if command == "" {
			command = strings.TrimSpace(entry.CommandTemplate)
		}
		command = resolveCommandExecutable(command)
		entry.Command = command
		entry.CommandTemplate = command
	}
	for index := range entry.Children {
		entry.Children[index] = normalizeMenuCommandPaths(entry.Children[index])
	}
	return entry
}

func resolveCommandExecutable(command string) string {
	return resolveCommandWith(command, resolveExecutableWithWhere)
}

func resolveCommandWith(command string, lookup func(string) string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}

	executable, arguments := splitCommandExecutable(command)
	if executable == "" || commandExecutableIsPath(executable) {
		return command
	}

	resolved := strings.TrimSpace(lookup(executable))
	if resolved == "" {
		return command
	}
	return quoteCommandExecutable(resolved) + arguments
}

func notifyShellAssociationChanged() {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	proc := shell32.NewProc("SHChangeNotify")
	const shcneAssocChanged = 0x08000000
	const shcnfIDList = 0x0000
	_, _, _ = proc.Call(shcneAssocChanged, shcnfIDList, 0, 0)
}
