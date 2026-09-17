//go:build windows && integration

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestFileAssociationRegistryRoundTrip(t *testing.T) {
	service := &FileAssociationService{}
	extension := fmt.Sprintf(".winadmintest%x", time.Now().UnixNano())
	command := `bun run`
	if _, err := service.Save(extension, command); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Delete(extension); err != nil {
			t.Errorf("清理测试扩展名失败: %v", err)
		}
	})
	commandKey, err := registry.OpenKey(registry.CURRENT_USER, associationClasses+associationProgID(extension)+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	registeredCommand, _, err := commandKey.GetStringValue("")
	commandKey.Close()
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := associationLauncherCommand(extension)
	if err != nil || registeredCommand != launcher {
		t.Fatalf("Shell 启动命令不符: %q, %v", registeredCommand, err)
	}
	if !strings.Contains(registeredCommand, associationLauncherExecutable) {
		t.Fatalf("Shell 应通过普通权限 launcher 启动: %q", registeredCommand)
	}
	progKey, err := registry.OpenKey(registry.CURRENT_USER, associationClasses+associationProgID(extension), registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	storedExecutable, _, err := progKey.GetStringValue(associationExecutableValue)
	progKey.Close()
	if err != nil || !filepath.IsAbs(storedExecutable) {
		t.Fatalf("执行程序路径无效: %q, %v", storedExecutable, err)
	}
	if _, err := os.Stat(storedExecutable); err != nil {
		t.Fatal(err)
	}
	items, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Extension == extension {
			if item.Command != command || item.Active || item.RegistryPath != associationRegistryPath(extension) {
				t.Fatalf("读取结果不符: %+v", item)
			}
			return
		}
	}
	t.Fatalf("未找到测试扩展名 %s", extension)
}

func TestFileAssociationRename(t *testing.T) {
	service := &FileAssociationService{}
	base := fmt.Sprintf("winadmintest%x", time.Now().UnixNano())
	previousExtension := "." + base + "a"
	extension := "." + base + "b"
	for _, value := range []string{previousExtension, extension} {
		t.Cleanup(func() {
			registered, err := associationIsRegistered(value)
			if err != nil {
				t.Error(err)
			} else if registered {
				if err := service.Delete(value); err != nil {
					t.Errorf("清理测试扩展名失败: %v", err)
				}
			}
		})
	}
	if _, err := service.Save(previousExtension, `bun run "%1"`); err != nil {
		t.Fatal(err)
	}
	item, err := service.Rename(previousExtension, strings.TrimPrefix(extension, "."), `msedge --app="%1"`)
	if err != nil {
		t.Fatal(err)
	}
	if item.Extension != extension || item.Command != `msedge --app="%1"` || item.Active {
		t.Fatalf("改名结果不符: %+v", item)
	}
	items, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range items {
		if candidate.Extension == previousExtension {
			t.Fatalf("旧扩展名仍在列表中: %+v", candidate)
		}
		if candidate.Extension == extension {
			if _, err := service.Save(previousExtension, `bun run "%1"`); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Rename(previousExtension, extension, `bun run "%1"`); err == nil {
				t.Fatal("目标扩展名已有打开方式时应拒绝改名")
			}
			return
		}
	}
	t.Fatalf("未找到新扩展名 %s", extension)
}
