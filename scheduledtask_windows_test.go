//go:build windows

package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRestartScheduledTaskWaitsForReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls []string
	states := []int{4, 2, 3}
	err := restartScheduledTask(ctx, func() error {
		calls = append(calls, "stop")
		return nil
	}, func() (int, error) {
		calls = append(calls, "state")
		current := states[0]
		states = states[1:]
		return current, nil
	}, func() error {
		calls = append(calls, "run")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"stop", "state", "state", "state", "run"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("调用顺序 = %v，期望 %v", calls, want)
	}
}

func TestRestartScheduledTaskFailures(t *testing.T) {
	failure := errors.New("接口失败")
	for _, test := range []struct {
		name     string
		stopErr  error
		stateErr error
		state    int
		timeout  bool
	}{
		{name: "停止失败", stopErr: failure},
		{name: "状态读取失败", stateErr: failure},
		{name: "已禁用", state: 1},
		{name: "未知状态", state: 0},
		{name: "停止超时", state: 4, timeout: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			stateRead, started := false, false
			err := restartScheduledTask(ctx, func() error { return test.stopErr }, func() (int, error) {
				stateRead = true
				return test.state, test.stateErr
			}, func() error {
				started = true
				return nil
			})
			if err == nil || started {
				t.Fatalf("失败后不能启动：error = %v，started = %v", err, started)
			}
			if test.stopErr != nil && stateRead {
				t.Fatal("停止失败后不应查询状态")
			}
			if test.timeout && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("期望超时错误，实际 %v", err)
			}
		})
	}
}

func TestRestartScheduledTaskRunFailure(t *testing.T) {
	failure := errors.New("启动失败")
	err := restartScheduledTask(context.Background(), func() error { return nil }, func() (int, error) {
		return 3, nil
	}, func() error { return failure })
	if !errors.Is(err, failure) {
		t.Fatalf("应返回启动错误，实际 %v", err)
	}
}

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
