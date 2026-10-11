//go:build windows

package main

import "testing"

func TestMarkDuplicatePathEntries(t *testing.T) {
	entries := []EnvironmentPathEntry{
		{Value: `%TOOLS%`, Expanded: `C:\Tools`},
		{Value: `c:/tools/`, Expanded: `c:/tools/`},
		{Expanded: ` "C:\Tools\." `},
		{Expanded: `C:\Tools\bin\..`},
		{Expanded: `C:\Other`},
		{Expanded: `C:\`},
		{Expanded: `c:/`},
		{Expanded: `C:`},
		{Expanded: `\\server\share\tools`},
		{Expanded: `//SERVER/share/tools/`},
		{Expanded: ""},
		{Expanded: " "},
	}
	want := []bool{false, true, true, true, false, false, true, false, false, true, false, false}
	markDuplicatePathEntries(entries)
	for index, entry := range entries {
		if entry.Duplicate != want[index] {
			t.Errorf("路径 %q: duplicate = %v, want %v", entry.Expanded, entry.Duplicate, want[index])
		}
	}
	// 删除首项后，首个剩余路径成为保留项。
	entries = entries[1:]
	markDuplicatePathEntries(entries)
	if entries[0].Duplicate || !entries[1].Duplicate {
		t.Fatal("删除首项后重复标记未重新计算")
	}
}
