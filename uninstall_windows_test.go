//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
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
