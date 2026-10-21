//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestPackedMSIProductCode(t *testing.T) {
	got, ok := packedMSIProductCode("{3300DF87-77D9-4D98-BC47-B8E12FB62A29}")
	if !ok {
		t.Fatal("expected valid MSI product code")
	}
	const want = "78FD00339D7789D4CB748B1EF26BA292"
	if got != want {
		t.Fatalf("packed MSI product code = %q, want %q", got, want)
	}
	if _, ok := packedMSIProductCode("not-a-guid"); ok {
		t.Fatal("invalid product code should be rejected")
	}
}

func TestFindInstalledAppExecutable(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"Reqable.exe", "unins000.exe", "ReqableHelper.exe"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got := findInstalledAppExecutable("Reqable version 3.2.23", directory)
	want := filepath.Join(directory, "Reqable.exe")
	if got != want {
		t.Fatalf("executable = %q, want %q", got, want)
	}
}

func TestInstalledAppRegistryPath(t *testing.T) {
	for _, test := range []struct {
		location uninstallRegistryLocation
		want     string
	}{
		{uninstallLocations[0], `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Demo`},
		{uninstallLocations[1], `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\Demo`},
		{uninstallLocations[2], `HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Demo`},
	} {
		if got := installedAppRegistryPath(test.location, "Demo"); got != test.want {
			t.Errorf("installedAppRegistryPath() = %q, want %q", got, test.want)
		}
	}
}

func TestInstalledAppIconsBatch(t *testing.T) {
	s := &AppUninstallService{}
	items, err := s.Icons(nil)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("空批次 = %+v，错误 = %v", items, err)
	}
	items, err = s.Icons([]string{"无效 ID", "另一个无效 ID"})
	if err != nil || len(items) != 2 || items[0].ID != "无效 ID" || items[1].ID != "另一个无效 ID" || items[0].Icon != "" || items[1].Icon != "" {
		t.Fatalf("图标缺失时应保留请求顺序和 ID：%+v，错误 = %v", items, err)
	}
	if _, err = s.Icons(make([]string, 129)); err == nil {
		t.Fatal("应拒绝超过 128 个图标的批次")
	}
}

func TestInstalledAppIconsFromRegistry(t *testing.T) {
	name := "win-admin-icon-test-" + filepath.Base(filepath.Dir(t.TempDir()))
	path := uninstallRegistryPath + `\` + name
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, path) })
	iconPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if err := key.SetStringValue("DisplayIcon", iconPath); err != nil {
		key.Close()
		t.Fatal(err)
	}
	key.Close()
	id := encodeInstalledAppID(uninstallLocations[2], name)
	items, err := (&AppUninstallService{}).Icons([]string{id})
	if err != nil || len(items) != 1 || items[0].ID != id || !strings.HasPrefix(items[0].Icon, "data:image/png;base64,") {
		t.Fatalf("应返回注册应用的 PNG 图标，错误 = %v", err)
	}
}
