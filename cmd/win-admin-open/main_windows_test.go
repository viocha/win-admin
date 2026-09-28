//go:build windows

package main

import "testing"

func TestCommandParts(t *testing.T) {
	filePath := `C:\test folder\sample.ts`
	parts, err := commandParts(`bun run`, filePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 || parts[0] != "bun" || parts[1] != "run" || parts[2] != filePath {
		t.Fatalf("省略占位符后的参数不符: %#v", parts)
	}

	parts, err = commandParts(`bun run "%1"`, filePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 || parts[2] != filePath {
		t.Fatalf("占位符替换结果不符: %#v", parts)
	}
}

func TestMenuCommandParts(t *testing.T) {
	path := `D:\tmp\中文 目录\a#b.html`
	parts, err := menuCommandParts(`msedge --app="%u" "%1"`, path)
	if err != nil || len(parts) != 3 || parts[1] != `--app=file:///D:/tmp/%E4%B8%AD%E6%96%87%20%E7%9B%AE%E5%BD%95/a%23b.html` || parts[2] != path {
		t.Fatalf("文件菜单参数不符: %#v, %v", parts, err)
	}
	parts, err = menuCommandParts(`tool --dir="%V" --url="%u"`, path)
	if err != nil || len(parts) != 3 || parts[1] != "--dir="+path || parts[2] != "--url=file:///D:/tmp/%E4%B8%AD%E6%96%87%20%E7%9B%AE%E5%BD%95/a%23b.html" {
		t.Fatalf("背景菜单参数不符: %#v, %v", parts, err)
	}
}
