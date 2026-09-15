//go:build windows

package main

import (
	"strings"
	"testing"
)

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
