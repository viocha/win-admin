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
