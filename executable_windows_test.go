//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLookupExecutablePath(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "current")
	first := filepath.Join(root, "first path")
	second := filepath.Join(root, "second")
	for _, path := range []string{current, first, second} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(current, "local.exe"), filepath.Join(first, "local.exe"),
		filepath.Join(first, "ordered.cmd"), filepath.Join(first, "ordered.exe"),
		filepath.Join(second, "ordered.exe"), filepath.Join(first, "custom.tool"),
	} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(current)
	t.Setenv("PATH", first+";"+second)
	t.Setenv("PATHEXT", ".CMD;.EXE;.TOOL")
	for _, test := range []struct{ name, want string }{
		{"local", filepath.Join(current, "local.exe")},
		{"ordered", filepath.Join(first, "ordered.cmd")},
		{"ordered.exe", filepath.Join(first, "ordered.exe")},
		{"custom", filepath.Join(first, "custom.tool")},
		{"missing", ""},
	} {
		if got := lookupExecutablePath(test.name); got != test.want {
			t.Errorf("查找 %q = %q，期望 %q", test.name, got, test.want)
		}
	}
	// PATH 中的相对目录也必须保存为绝对路径。
	t.Setenv("PATH", `..\first path`)
	if got := lookupExecutablePath("ordered.exe"); got != filepath.Join(first, "ordered.exe") {
		t.Fatalf("相对 PATH 解析结果 = %q", got)
	}
}
