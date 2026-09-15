//go:build windows

package main

import "testing"

func TestResolveExecutablePathWith(t *testing.T) {
	lookup := func(name string) string {
		switch name {
		case "pwsh":
			return `D:\Program Files\PowerShell\7\pwsh.exe`
		case "git":
			return `D:\Tools\git.exe`
		default:
			return ""
		}
	}

	tests := []struct {
		input string
		want  string
	}{
		{"pwsh", `D:\Program Files\PowerShell\7\pwsh.exe`},
		{`"pwsh"`, `D:\Program Files\PowerShell\7\pwsh.exe`},
		{"git", `D:\Tools\git.exe`},
		{`"C:\Program Files\PowerShell\7\pwsh.exe"`, `C:\Program Files\PowerShell\7\pwsh.exe`},
		{`%ComSpec%`, `%ComSpec%`},
		{"missing.exe", "missing.exe"},
	}

	for _, tt := range tests {
		if got := resolveExecutablePathWith(tt.input, lookup); got != tt.want {
			t.Errorf("resolveExecutablePathWith(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeScheduledTaskSettingsRestartInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		wantErr  bool
	}{
		{name: "ten seconds", interval: "PT10S", wantErr: true},
		{name: "one minute", interval: "PT1M", wantErr: false},
		{name: "ninety seconds", interval: "PT90S", wantErr: false},
		{name: "thirty one days", interval: "P31D", wantErr: false},
		{name: "over thirty one days", interval: "P31DT1S", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := defaultScheduledTaskSettings()
			settings.RestartCount = 1
			settings.RestartInterval = test.interval

			_, err := normalizeScheduledTaskSettings(settings)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeScheduledTaskSettings() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestNormalizeScheduledTaskSettingsRestartCountMaximum(t *testing.T) {
	settings := defaultScheduledTaskSettings()
	settings.RestartInterval = "PT1M"
	settings.RestartCount = 256

	if _, err := normalizeScheduledTaskSettings(settings); err == nil {
		t.Fatal("normalizeScheduledTaskSettings() expected an error for restart count 256")
	}
}
