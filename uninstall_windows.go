//go:build windows

package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const uninstallRegistryPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

type uninstallRegistryLocation struct {
	root         registry.Key
	scope        string
	architecture string
	view         uint32
}

var uninstallLocations = []uninstallRegistryLocation{
	{registry.LOCAL_MACHINE, "system", "x64", registry.WOW64_64KEY},
	{registry.LOCAL_MACHINE, "system", "x86", registry.WOW64_32KEY},
	{registry.CURRENT_USER, "user", "x64", registry.WOW64_64KEY},
	{registry.CURRENT_USER, "user", "x86", registry.WOW64_32KEY},
}

var installedAppIconPreviews sync.Map
var installedAppDirectorySizes sync.Map

func (s *AppUninstallService) List() ([]InstalledApp, error) {
	apps := make([]InstalledApp, 0, 128)
	seen := make(map[string]struct{}, 128)
	for _, location := range uninstallLocations {
		items, err := listInstalledAppsAt(location)
		if err != nil {
			continue
		}
		for _, item := range items {
			key := strings.ToLower(item.Name + "\x00" + item.Version + "\x00" + item.Publisher + "\x00" + item.InstallLocation)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			apps = append(apps, item)
		}
	}
	sort.SliceStable(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
	return apps, nil
}

func (s *AppUninstallService) Sizes(ids []string) []InstalledAppSize {
	if len(ids) == 0 {
		return []InstalledAppSize{}
	}

	type job struct {
		index int
		id    string
	}
	results := make([]InstalledAppSize, len(ids))
	valid := make([]bool, len(ids))
	jobs := make(chan job)
	var wg sync.WaitGroup
	workerCount := 4
	if len(ids) < workerCount {
		workerCount = len(ids)
	}
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for current := range jobs {
				size, ok := installedAppSize(current.id)
				if !ok {
					continue
				}
				results[current.index] = InstalledAppSize{ID: current.id, EstimatedSizeKB: size}
				valid[current.index] = true
			}
		}()
	}
	for index, id := range ids {
		jobs <- job{index: index, id: id}
	}
	close(jobs)
	wg.Wait()

	resolved := make([]InstalledAppSize, 0, len(ids))
	for index, item := range results {
		if valid[index] {
			resolved = append(resolved, item)
		}
	}
	return resolved
}

func (s *AppUninstallService) Uninstall(id string) error {
	location, subkeyName, err := decodeInstalledAppID(id)
	if err != nil {
		return err
	}
	key, err := registry.OpenKey(location.root, uninstallRegistryPath+`\`+subkeyName, registry.QUERY_VALUE|location.view)
	if err != nil {
		return fmt.Errorf("读取应用卸载信息失败: %w", err)
	}
	defer key.Close()

	command := readUninstallString(key, "UninstallString")
	if command == "" {
		command = readUninstallString(key, "QuietUninstallString")
	}
	if command == "" {
		return errors.New("应用未提供卸载命令")
	}
	return startWindowsCommand(command)
}

func (s *AppUninstallService) OpenInstallLocation(id string) error {
	location, subkeyName, err := decodeInstalledAppID(id)
	if err != nil {
		return err
	}
	key, err := registry.OpenKey(location.root, uninstallRegistryPath+`\`+subkeyName, registry.QUERY_VALUE|location.view)
	if err != nil {
		return fmt.Errorf("读取安装位置失败: %w", err)
	}
	installLocation := inferInstallLocation(
		readUninstallString(key, "InstallLocation"),
		readUninstallString(key, "DisplayIcon"),
		readUninstallString(key, "UninstallString"),
		readUninstallString(key, "QuietUninstallString"),
	)
	key.Close()
	if installLocation == "" {
		return errors.New("找不到安装位置")
	}
	path, err := windows.UTF16PtrFromString(installLocation)
	if err != nil {
		return fmt.Errorf("安装位置无效: %w", err)
	}
	if err := windows.ShellExecute(0, nil, path, nil, nil, 1); err != nil {
		return fmt.Errorf("打开安装位置失败: %w", err)
	}
	return nil
}

func (s *AppUninstallService) OpenRegistry(id string) error {
	location, subkeyName, err := decodeInstalledAppID(id)
	if err != nil {
		return err
	}
	key, err := registry.OpenKey(location.root, uninstallRegistryPath+`\`+subkeyName, registry.QUERY_VALUE|location.view)
	if err != nil {
		return fmt.Errorf("读取应用注册表项失败: %w", err)
	}
	key.Close()

	return openRegistryKey(installedAppRegistryPath(location, subkeyName))
}

func installedAppRegistryPath(location uninstallRegistryLocation, subkeyName string) string {
	hive := `HKEY_CURRENT_USER`
	path := uninstallRegistryPath
	if location.scope == "system" {
		hive = `HKEY_LOCAL_MACHINE`
		if location.architecture == "x86" {
			path = strings.Replace(path, `SOFTWARE\`, `SOFTWARE\WOW6432Node\`, 1)
		}
	}
	return hive + `\` + path + `\` + subkeyName
}

func listInstalledAppsAt(location uninstallRegistryLocation) ([]InstalledApp, error) {
	key, err := registry.OpenKey(location.root, uninstallRegistryPath, registry.ENUMERATE_SUB_KEYS|location.view)
	if errors.Is(err, registry.ErrNotExist) {
		return []InstalledApp{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer key.Close()

	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	apps := make([]InstalledApp, 0, len(names))
	for _, subkeyName := range names {
		item, itemErr := readInstalledApp(location, subkeyName)
		if itemErr == nil && item.Name != "" {
			apps = append(apps, item)
		}
	}
	return apps, nil
}

func readInstalledApp(location uninstallRegistryLocation, subkeyName string) (InstalledApp, error) {
	key, err := registry.OpenKey(location.root, uninstallRegistryPath+`\`+subkeyName, registry.QUERY_VALUE|location.view)
	if err != nil {
		return InstalledApp{}, err
	}
	defer key.Close()

	name := strings.TrimSpace(readUninstallString(key, "DisplayName"))
	if name == "" || readUninstallDWORD(key, "SystemComponent") == 1 {
		return InstalledApp{}, errors.New("不是可卸载应用")
	}
	if strings.TrimSpace(readUninstallString(key, "ParentKeyName")) != "" {
		return InstalledApp{}, errors.New("跳过子组件")
	}
	if readUninstallString(key, "UninstallString") == "" && readUninstallString(key, "QuietUninstallString") == "" {
		return InstalledApp{}, errors.New("应用未提供卸载命令")
	}
	displayIcon := strings.TrimSpace(readUninstallString(key, "DisplayIcon"))
	uninstallString := strings.TrimSpace(readUninstallString(key, "UninstallString"))
	quietUninstallString := strings.TrimSpace(readUninstallString(key, "QuietUninstallString"))
	installLocation := inferInstallLocation(
		readUninstallString(key, "InstallLocation"),
		displayIcon,
		uninstallString,
		quietUninstallString,
	)
	installDate := normalizeInstallDate(readUninstallString(key, "InstallDate"))
	var installTimestamp int64
	if info, statErr := key.Stat(); statErr == nil && !info.ModTime().IsZero() {
		modifiedAt := info.ModTime()
		modifiedDate := modifiedAt.Format("2006-01-02")
		if installDate == "" {
			installDate = modifiedDate
		}
		// InstallDate 通常只有日期。仅当注册表项修改日期与安装日期一致时，
		// 才把修改时间作为同一天内排序的辅助依据，避免后续更新影响排序。
		if installDate == modifiedDate {
			installTimestamp = modifiedAt.UnixMilli()
		}
	}
	return InstalledApp{
		ID:   encodeInstalledAppID(location, subkeyName),
		Name: name,
		Icon: installedAppIconPreview(
			displayIcon,
			msiProductIcon(subkeyName),
			findInstalledAppExecutable(name, installLocation),
		),
		Version:          strings.TrimSpace(readUninstallString(key, "DisplayVersion")),
		Publisher:        strings.TrimSpace(readUninstallString(key, "Publisher")),
		InstallDate:      installDate,
		InstallTimestamp: installTimestamp,
		EstimatedSizeKB:  readUninstallDWORD(key, "EstimatedSize"),
		InstallLocation:  installLocation,
		RegistryPath:     installedAppRegistryPath(location, subkeyName),
		Scope:            location.scope,
		Architecture:     location.architecture,
	}, nil
}

func installedAppSize(id string) (uint64, bool) {
	location, subkeyName, err := decodeInstalledAppID(id)
	if err != nil {
		return 0, false
	}
	key, err := registry.OpenKey(location.root, uninstallRegistryPath+`\\`+subkeyName, registry.QUERY_VALUE|location.view)
	if err != nil {
		return 0, false
	}
	defer key.Close()

	if size := readUninstallDWORD(key, "EstimatedSize"); size != 0 {
		return size, true
	}
	installLocation := inferInstallLocation(
		readUninstallString(key, "InstallLocation"),
		readUninstallString(key, "DisplayIcon"),
		readUninstallString(key, "UninstallString"),
		readUninstallString(key, "QuietUninstallString"),
	)
	if installLocation == "" {
		return 0, true
	}
	return installedAppDirectorySizeKB(installLocation), true
}

func inferInstallLocation(explicit, displayIcon, uninstallString, quietUninstallString string) string {
	if path := normalizeInstallDirectory(explicit); path != "" {
		return path
	}
	for _, source := range []string{displayIcon, uninstallString, quietUninstallString} {
		candidate := installedAppSourcePath(source)
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil {
			if info.IsDir() {
				if path := normalizeInstallDirectory(candidate); path != "" {
					return path
				}
			} else if path := normalizeInstallDirectory(filepath.Dir(candidate)); path != "" {
				return path
			}
		}
	}
	return ""
}

func installedAppSourcePath(source string) string {
	source = strings.TrimSpace(os.ExpandEnv(source))
	if source == "" {
		return ""
	}
	if strings.HasPrefix(source, `"`) {
		if end := strings.Index(source[1:], `"`); end >= 0 {
			return filepath.Clean(source[1 : end+1])
		}
	}
	if info, err := os.Stat(strings.Trim(source, `"`)); err == nil && !info.IsDir() {
		return filepath.Clean(strings.Trim(source, `"`))
	}
	lower := strings.ToLower(source)
	for _, extension := range []string{".exe", ".com", ".bat", ".cmd", ".ico", ".dll"} {
		if end := strings.Index(lower, extension); end >= 0 {
			candidate := strings.Trim(strings.TrimSpace(source[:end+len(extension)]), `"`)
			if candidate != "" {
				return filepath.Clean(candidate)
			}
		}
	}
	if comma := strings.LastIndex(source, ","); comma > 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(source[comma+1:])); err == nil {
			return filepath.Clean(strings.Trim(strings.TrimSpace(source[:comma]), `"`))
		}
	}
	return ""
}

func normalizeInstallDirectory(path string) string {
	path = strings.TrimSpace(os.ExpandEnv(strings.Trim(path, `"`)))
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() || !usefulInstallDirectory(clean) {
		return ""
	}
	return clean
}

func usefulInstallDirectory(path string) bool {
	clean := strings.TrimRight(filepath.Clean(path), `\\/`)
	if clean == "" || strings.EqualFold(clean+`\\`, filepath.VolumeName(clean)+`\\`) {
		return false
	}
	for _, broad := range []string{
		os.Getenv("SystemRoot"),
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("ProgramData"),
		os.Getenv("USERPROFILE"),
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("APPDATA"),
	} {
		if broad != "" && strings.EqualFold(clean, strings.TrimRight(filepath.Clean(broad), `\\/`)) {
			return false
		}
	}
	return true
}

func installedAppDirectorySizeKB(path string) uint64 {
	cacheKey := strings.ToLower(filepath.Clean(path))
	if cached, ok := installedAppDirectorySizes.Load(cacheKey); ok {
		return cached.(uint64)
	}
	var size uint64
	_ = filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr == nil && info.Size() > 0 {
			size += uint64(info.Size())
		}
		return nil
	})
	result := (size + 1023) / 1024
	installedAppDirectorySizes.Store(cacheKey, result)
	return result
}

func installedAppIconPreview(sources ...string) string {
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		cacheKey := strings.ToLower(source)
		if _, exists := seen[cacheKey]; exists {
			continue
		}
		seen[cacheKey] = struct{}{}

		if cached, ok := installedAppIconPreviews.Load(cacheKey); ok {
			if preview := cached.(string); preview != "" {
				return preview
			}
			continue
		}
		preview, err := iconPreviewDataURL(source, 32)
		if err != nil {
			installedAppIconPreviews.Store(cacheKey, "")
			continue
		}
		installedAppIconPreviews.Store(cacheKey, preview)
		return preview
	}
	return ""
}

func msiProductIcon(uninstallSubkey string) string {
	productKey, ok := packedMSIProductCode(uninstallSubkey)
	if !ok {
		return ""
	}
	key, err := registry.OpenKey(registry.CLASSES_ROOT, `Installer\Products\`+productKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	return strings.TrimSpace(readUninstallString(key, "ProductIcon"))
}

func packedMSIProductCode(value string) (string, bool) {
	value = strings.TrimSpace(strings.Trim(value, "{}"))
	parts := strings.Split(value, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return "", false
	}
	for _, part := range parts {
		for _, char := range part {
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return "", false
			}
		}
	}
	return strings.ToUpper(
		reverseASCII(parts[0]) +
			reverseASCII(parts[1]) +
			reverseASCII(parts[2]) +
			swapHexPairs(parts[3]) +
			swapHexPairs(parts[4]),
	), true
}

func reverseASCII(value string) string {
	bytes := []byte(value)
	for left, right := 0, len(bytes)-1; left < right; left, right = left+1, right-1 {
		bytes[left], bytes[right] = bytes[right], bytes[left]
	}
	return string(bytes)
}

func swapHexPairs(value string) string {
	bytes := []byte(value)
	for index := 0; index+1 < len(bytes); index += 2 {
		bytes[index], bytes[index+1] = bytes[index+1], bytes[index]
	}
	return string(bytes)
}

func findInstalledAppExecutable(name, installLocation string) string {
	directory := normalizeInstallDirectory(installLocation)
	if directory == "" {
		return ""
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ""
	}

	appName := installedAppMatchName(name)
	bestPath := ""
	bestScore := -1
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".exe") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		candidate := normalizeAppName(base)
		if candidate == "" || isAuxiliaryExecutable(candidate) {
			continue
		}

		score := 0
		switch {
		case candidate == appName && appName != "":
			score = 100
		case appName != "" && strings.HasPrefix(candidate, appName):
			score = 80
		case appName != "" && strings.HasPrefix(appName, candidate) && len(candidate) >= 4:
			score = 70
		case appName != "" && strings.Contains(candidate, appName):
			score = 60
		}
		if score > bestScore {
			bestScore = score
			bestPath = filepath.Join(directory, entry.Name())
		}
	}
	if bestScore <= 0 {
		return ""
	}
	return bestPath
}

func installedAppMatchName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, marker := range []string{" version ", " 版本 "} {
		if index := strings.Index(lower, marker); index >= 0 {
			name = name[:index]
			break
		}
	}
	return normalizeAppName(name)
}

func normalizeAppName(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(value) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func isAuxiliaryExecutable(name string) bool {
	for _, marker := range []string{"unins", "uninstall", "setup", "update", "updater", "helper", "service", "crash", "repair"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func readUninstallString(key registry.Key, name string) string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

func readUninstallDWORD(key registry.Key, name string) uint64 {
	value, _, err := key.GetIntegerValue(name)
	if err != nil {
		return 0
	}
	return value
}

func normalizeInstallDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{
		"20060102",
		"2006-01-02",
		time.ANSIC,
		time.RFC1123,
		time.RFC1123Z,
		"Jan 2 2006",
		"January 2 2006",
		"01/02/2006",
		"1/2/2006",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return value
}

func encodeInstalledAppID(location uninstallRegistryLocation, subkeyName string) string {
	payload := location.scope + "\x00" + location.architecture + "\x00" + subkeyName
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeInstalledAppID(id string) (uninstallRegistryLocation, string, error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(id))
	if err != nil {
		return uninstallRegistryLocation{}, "", errors.New("应用标识无效")
	}
	parts := strings.SplitN(string(data), "\x00", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[2]) == "" {
		return uninstallRegistryLocation{}, "", errors.New("应用标识无效")
	}
	for _, location := range uninstallLocations {
		if location.scope == parts[0] && location.architecture == parts[1] {
			return location, parts[2], nil
		}
	}
	return uninstallRegistryLocation{}, "", errors.New("应用标识无效")
}

const (
	jobCompletionKey              uintptr = 1
	jobObjectMsgActiveProcessZero uint32  = 4
)

type jobObjectAssociateCompletionPort struct {
	CompletionKey  uintptr
	CompletionPort windows.Handle
}

func startWindowsCommand(command string) error {
	commandLine, err := windows.UTF16PtrFromString(command)
	if err != nil {
		return fmt.Errorf("卸载命令无效: %w", err)
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("创建卸载任务失败: %w", err)
	}
	defer windows.CloseHandle(job)

	startupInfo := &windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var processInfo windows.ProcessInformation
	if err := windows.CreateProcess(
		nil,
		commandLine,
		nil,
		nil,
		false,
		windows.CREATE_NEW_PROCESS_GROUP|windows.CREATE_SUSPENDED,
		nil,
		nil,
		startupInfo,
		&processInfo,
	); err != nil {
		return fmt.Errorf("启动卸载程序失败: %w", err)
	}
	defer windows.CloseHandle(processInfo.Thread)
	defer windows.CloseHandle(processInfo.Process)

	if err := windows.AssignProcessToJobObject(job, processInfo.Process); err != nil {
		_ = windows.TerminateProcess(processInfo.Process, 1)
		return fmt.Errorf("跟踪卸载程序失败: %w", err)
	}

	completionPort, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		_ = windows.TerminateProcess(processInfo.Process, 1)
		return fmt.Errorf("创建卸载任务事件端口失败: %w", err)
	}
	defer windows.CloseHandle(completionPort)

	completionInfo := jobObjectAssociateCompletionPort{
		CompletionKey:  jobCompletionKey,
		CompletionPort: completionPort,
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectAssociateCompletionPortInformation,
		uintptr(unsafe.Pointer(&completionInfo)),
		uint32(unsafe.Sizeof(completionInfo)),
	); err != nil {
		_ = windows.TerminateProcess(processInfo.Process, 1)
		return fmt.Errorf("监听卸载任务失败: %w", err)
	}

	if _, err := windows.ResumeThread(processInfo.Thread); err != nil {
		_ = windows.TerminateProcess(processInfo.Process, 1)
		return fmt.Errorf("启动卸载程序失败: %w", err)
	}

	return waitForJobProcesses(completionPort)
}

func waitForJobProcesses(completionPort windows.Handle) error {
	for {
		var message uint32
		var key uintptr
		var overlapped *windows.Overlapped
		if err := windows.GetQueuedCompletionStatus(
			completionPort,
			&message,
			&key,
			&overlapped,
			windows.INFINITE,
		); err != nil {
			return fmt.Errorf("等待卸载程序结束失败: %w", err)
		}
		if key == jobCompletionKey && message == jobObjectMsgActiveProcessZero {
			return nil
		}
	}
}
