//go:build windows

package fileassociation

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// CommandParts 将打开命令中的文件占位符替换为路径或 file URL。
func CommandParts(command, filePath string) ([]string, error) {
	args, err := windows.DecomposeCommandLine(command)
	if err != nil || len(args) == 0 || args[0] == "" {
		return nil, errors.New("打开命令格式无效")
	}

	fileURL := FileURL(filePath)
	replacer := strings.NewReplacer("%1", filePath, "%u", fileURL)
	hasFile := false
	for index := 1; index < len(args); index++ {
		if strings.Contains(args[index], "%1") || strings.Contains(args[index], "%u") {
			hasFile = true
		}
		args[index] = replacer.Replace(args[index])
	}
	if !hasFile {
		args = append(args, filePath)
	}
	return args, nil
}

// FileURL 把 Windows 文件路径转换为可传给浏览器的 file URL。
func FileURL(filePath string) string {
	path := filepath.ToSlash(filePath)
	if strings.HasPrefix(path, "//") {
		parts := strings.SplitN(strings.TrimPrefix(path, "//"), "/", 2)
		if len(parts) == 2 {
			return (&url.URL{Scheme: "file", Host: parts[0], Path: "/" + parts[1]}).String()
		}
	}
	return (&url.URL{Scheme: "file", Path: "/" + path}).String()
}
