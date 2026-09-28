//go:build windows

package main

import (
	"os"
	"strings"
	"testing"
)

func TestResolveMenuLabel(t *testing.T) {
	if got := resolveMenuLabel("普通菜单"); got != "普通菜单" {
		t.Fatalf("普通名称被修改: %q", got)
	}
	const dll = `C:\Program Files (x86)\Common Files\Microsoft Shared\MSEnv\2052\VSLauncherUI.dll`
	if _, err := os.Stat(dll); err != nil {
		t.Skip("本机没有 VSLauncherUI.dll")
	}
	const indirect = `@` + dll + `,-1002`
	resolved := resolveMenuLabel(indirect)
	if resolved == "" || resolved == indirect {
		t.Fatalf("未解析间接资源字符串: %q", resolved)
	}
	t.Logf("%s => %s", indirect, resolved)
}

func TestFirstCommandToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"C:\Program Files\Demo\demo.exe" "%1"`, `C:\Program Files\Demo\demo.exe`},
		{`notepad.exe "%1"`, `notepad.exe`},
		{"", ""},
	}

	for _, tt := range tests {
		if got := firstCommandToken(tt.input); got != tt.want {
			t.Errorf("firstCommandToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveCommandWith(t *testing.T) {
	lookup := func(name string) string {
		if name == "pwsh" {
			return `D:\Program Files\PowerShell\7\pwsh.exe`
		}
		if name == "git" {
			return `D:\Tools\git.exe`
		}
		return ""
	}

	tests := []struct {
		input string
		want  string
	}{
		{`pwsh -WorkingDirectory "%V"`, `"D:\Program Files\PowerShell\7\pwsh.exe" -WorkingDirectory "%V"`},
		{`"pwsh" -NoLogo`, `"D:\Program Files\PowerShell\7\pwsh.exe" -NoLogo`},
		{`git status`, `D:\Tools\git.exe status`},
		{`"C:\Program Files\PowerShell\7\pwsh.exe" -NoLogo`, `"C:\Program Files\PowerShell\7\pwsh.exe" -NoLogo`},
		{`%ComSpec% /c echo test`, `%ComSpec% /c echo test`},
		{`missing.exe "%1"`, `missing.exe "%1"`},
	}

	for _, tt := range tests {
		if got := resolveCommandWith(tt.input, lookup); got != tt.want {
			t.Errorf("resolveCommandWith(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidVerbID(t *testing.T) {
	valid := []string{"OpenWithCode", "Open With Code", "win-admin.demo.1"}
	for _, value := range valid {
		if !validVerbID(value) {
			t.Errorf("expected %q to be valid", value)
		}
	}

	invalid := []string{"", ".", "..", `a\b`, "a/b"}
	for _, value := range invalid {
		if validVerbID(value) {
			t.Errorf("expected %q to be invalid", value)
		}
	}
}

func TestBackgroundMenuSourcesAreSeparate(t *testing.T) {
	for _, test := range []struct {
		target string
		want   string
		other  string
	}{
		{"background", `\Directory\Background\`, `\DesktopBackground\`},
		{"desktop-background", `\DesktopBackground\`, `\Directory\Background\`},
	} {
		sources, err := menuSourcesForTarget(test.target)
		if err != nil {
			t.Fatal(err)
		}
		if len(sources) == 0 || !strings.Contains(sources[0].path, test.want) || sources[0].readOnly {
			t.Fatalf("%s 的可编辑主路径不符: %#v", test.target, sources)
		}
		for _, source := range sources {
			if strings.Contains(source.path, test.other) {
				t.Errorf("%s 混入其他背景路径: %s", test.target, source.path)
			}
		}
	}
}

func TestMenuRegistryCommand(t *testing.T) {
	const path = `Software\Classes\Directory\Background\shell\Example`
	plain, err := menuRegistryCommand(`pwsh -WorkingDirectory "%V"`, "background", path)
	if err != nil || plain != `pwsh -WorkingDirectory "%V"` {
		t.Fatalf("普通菜单命令被改写: %q, %v", plain, err)
	}
	wrapped, err := menuRegistryCommand(`msedge --app="%u"`, "background", path)
	if err != nil || !strings.Contains(wrapped, ` --menu "`+path+`\command" "%V"`) {
		t.Fatalf("URL 菜单命令未传递背景路径: %q, %v", wrapped, err)
	}
}

func TestMenuExtensionsAppliesTo(t *testing.T) {
	got, err := menuExtensionsAppliesTo("EXE, *.ps1 exe")
	if err != nil {
		t.Fatal(err)
	}
	want := "System.FileExtension:=.exe OR System.FileExtension:=.ps1"
	if got != want {
		t.Fatalf("menuExtensionsAppliesTo() = %q, want %q", got, want)
	}
	if parsed := parseMenuExtensions(got); parsed != "exe, ps1" {
		t.Fatalf("parseMenuExtensions() = %q, want %q", parsed, "exe, ps1")
	}
	if _, err := menuExtensionsAppliesTo("exe, bad/ext"); err == nil {
		t.Fatal("非法扩展名应报错")
	}
}

func TestParseIconLocation(t *testing.T) {
	path, index, err := parseIconLocation(`"%SystemRoot%\System32\shell32.dll",-42`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.ToLower(path), `\system32\shell32.dll`) {
		t.Fatalf("unexpected icon path: %q", path)
	}
	if index != -42 {
		t.Fatalf("unexpected icon index: %d", index)
	}
}

func TestIconPreviewDataURL(t *testing.T) {
	preview, err := iconPreviewDataURL(`%SystemRoot%\System32\shell32.dll,0`, 32)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(preview, "data:image/png;base64,") {
		t.Fatalf("unexpected preview prefix: %.32q", preview)
	}
}
