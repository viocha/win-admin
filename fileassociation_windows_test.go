//go:build windows

package main

import (
	"os"
	"testing"
)

func TestNormalizeAssociationExtension(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{" TS ", ".ts"},
		{".HTML", ".html"},
	} {
		got, err := normalizeAssociationExtension(test.input)
		if err != nil || got != test.want {
			t.Errorf("normalizeAssociationExtension(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
	for _, input := range []string{"", ".", `..ts`, `a\b`, `.ts\x`} {
		if _, err := normalizeAssociationExtension(input); err == nil {
			t.Errorf("expected %q to be rejected", input)
		}
	}
}

func TestNormalizeAssociationCommand(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{`bun run`, `bun run`},
		{`msedge --app="%1"`, `msedge --app="%1"`},
	} {
		got, err := normalizeAssociationCommand(test.input)
		if err != nil || got != test.want {
			t.Errorf("normalizeAssociationCommand(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
	if _, err := normalizeAssociationCommand(" "); err == nil {
		t.Error("empty command should be rejected")
	}
	if _, err := normalizeAssociationCommand("bun run\ncalc.exe"); err == nil {
		t.Error("multiline command should be rejected")
	}
}

func TestAssociationCommandArgs(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	filePath := `C:\test folder\sample.ts`
	gotExecutable, args, err := associationCommandArgs(`"`+executable+`" --app="%1" "%1"`, filePath)
	if err != nil {
		t.Fatal(err)
	}
	if gotExecutable != executable || len(args) != 2 || args[0] != "--app="+filePath || args[1] != filePath {
		t.Fatalf("命令参数不符: %q, %#v", gotExecutable, args)
	}
	parts, err := associationCommandParts(`bun run`, filePath)
	if err != nil || len(parts) != 3 || parts[1] != "run" || parts[2] != filePath {
		t.Fatalf("省略占位符后的参数不符: %#v, %v", parts, err)
	}
	if _, _, err := associationCommandArgs(`missing-win-admin-program "%1"`, filePath); err == nil {
		t.Fatal("找不到程序时应报错")
	}
}
