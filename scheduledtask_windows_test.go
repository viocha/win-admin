//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

func TestWaitScheduledTaskWaitsForCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	calls := 0
	err := waitScheduledTask(ctx, "等待失败", func() (bool, error) {
		calls++
		return calls == 3, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("检查次数 = %d，期望 3", calls)
	}
}

func TestScheduledTaskWrapperRestartIntegration(t *testing.T) {
	wrapper := os.Getenv("WIN_ADMIN_TEST_RUN_HIDDEN")
	if wrapper == "" {
		t.Skip("设置 WIN_ADMIN_TEST_RUN_HIDDEN 运行包装器重启测试")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"start":"bun run server.ts"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	code := fmt.Sprintf(`import {writeFileSync} from "node:fs"; try {Bun.serve({hostname:"127.0.0.1",port:%d,fetch:()=>new Response(String(process.pid))}); writeFileSync("ready.txt",String(process.pid));} catch(error) {writeFileSync("error.txt",String(error)); throw error;}`, port)
	if err := os.WriteFile(filepath.Join(dir, "server.ts"), []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	s := &ScheduledTaskService{}
	settings := defaultScheduledTaskSettings()
	settings.RestartCount = 0
	details, err := s.Save(SaveScheduledTaskRequest{Task: ScheduledTaskEdit{
		Name: fmt.Sprintf("win-admin-test-wrapper-%d", time.Now().UnixNano()), Folder: `\`, Enabled: true,
		LogonType: taskLogonInteractiveToken, RunLevel: "least",
		Action:  ScheduledTaskAction{Command: wrapper, Arguments: "bun run start", WorkingDirectory: dir},
		Trigger: ScheduledTaskTrigger{Type: "manual"}, Settings: settings,
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := details.Summary.Path
	t.Cleanup(func() {
		if _, err := s.Stop(path, ""); err != nil {
			t.Error(err)
		}
		if err := s.Delete(path); err != nil {
			t.Error(err)
		}
	})
	waitReady := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := waitScheduledTask(ctx, "等待测试服务监听", func() (bool, error) {
			_, err := os.ReadFile(filepath.Join(dir, "ready.txt"))
			if err == nil {
				return true, nil
			}
			items, readErr := s.Summaries([]string{path})
			if readErr != nil {
				return false, readErr
			}
			if len(items) == 1 && items[0].State == "ready" && items[0].LastResult == 1 {
				message, _ := os.ReadFile(filepath.Join(dir, "error.txt"))
				return false, fmt.Errorf("包装器退出 1: %s", message)
			}
			return false, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Run(path, ""); err != nil {
		t.Fatal(err)
	}
	waitReady()
	for index := range 10 {
		os.Remove(filepath.Join(dir, "ready.txt"))
		var states []string
		result, err := controlScheduledTask(path, "restart", func(summary ScheduledTaskSummary) { states = append(states, summary.State) })
		if err != nil {
			t.Fatalf("第 %d 次重启：%v", index+1, err)
		}
		t.Logf("第 %d 次重启返回：%s", index+1, result.State)
		ready, running := -1, -1
		for index, state := range states {
			if state == "ready" && ready == -1 {
				ready = index
			}
			if state == "running" {
				running = index
			}
		}
		if ready == -1 || running <= ready {
			t.Fatalf("状态阶段顺序错误: %v", states)
		}
		waitReady()
	}
}

func TestWaitScheduledTaskFailures(t *testing.T) {
	failure := errors.New("接口失败")
	for _, test := range []struct {
		name     string
		checkErr error
	}{
		{name: "状态读取失败", checkErr: failure},
		{name: "操作超时"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			err := waitScheduledTask(ctx, "等待失败", func() (bool, error) {
				return false, test.checkErr
			})
			want := test.checkErr
			if want == nil {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) {
				t.Fatalf("期望 %v，实际 %v", want, err)
			}
		})
	}
}

func TestScheduledTaskStartConfirmed(t *testing.T) {
	for _, test := range []struct {
		name          string
		instanceState int
		state         string
		result        int32
		done          bool
		wantErr       bool
	}{
		{name: "新实例运行中", instanceState: 4, state: "running", result: 0x41301, done: true},
		{name: "等待排队实例", instanceState: 2, state: "queued"},
		{name: "等待注册状态同步", instanceState: 4, state: "ready", result: 0},
		{name: "短任务正常完成", instanceState: 3, state: "ready", done: true},
		{name: "实例消失后正常完成", instanceState: 0, state: "ready", done: true},
		{name: "短任务执行失败", instanceState: 3, state: "ready", result: 1, wantErr: true},
		{name: "等待本次执行结果", instanceState: 0, state: "ready", result: 0x41306},
	} {
		t.Run(test.name, func(t *testing.T) {
			done, err := scheduledTaskStartConfirmed(test.instanceState, ScheduledTaskSummary{State: test.state, LastResult: test.result})
			if done != test.done || (err != nil) != test.wantErr {
				t.Fatalf("完成 = %v，错误 = %v", done, err)
			}
		})
	}
}

// 显式启用后仅创建并清理本测试的临时任务，不操作已有任务。
func TestScheduledTaskControlIntegration(t *testing.T) {
	if os.Getenv("WIN_ADMIN_TEST_TASK_SCHEDULER") != "1" {
		t.Skip("设置 WIN_ADMIN_TEST_TASK_SCHEDULER=1 运行真实调度器测试")
	}
	s := &ScheduledTaskService{}
	newTask := func(command, arguments string, allowStart bool) string {
		t.Helper()
		settings := defaultScheduledTaskSettings()
		settings.RestartCount = 0
		settings.AllowStartOnDemand = allowStart
		edit := ScheduledTaskEdit{
			Name: fmt.Sprintf("win-admin-test-%d", time.Now().UnixNano()), Folder: `\`, Enabled: true,
			LogonType: taskLogonInteractiveToken, RunLevel: "least",
			Action:  ScheduledTaskAction{Command: command, Arguments: arguments},
			Trigger: ScheduledTaskTrigger{Type: "manual"}, Settings: settings,
		}
		details, err := s.Save(SaveScheduledTaskRequest{Task: edit})
		if err != nil {
			t.Fatal(err)
		}
		path := details.Summary.Path
		t.Cleanup(func() {
			if _, err := s.Stop(path, ""); err != nil {
				t.Error(err)
			}
			if err := s.Delete(path); err != nil {
				t.Error(err)
			}
		})
		return path
	}

	t.Run("重启更换实例且停止后立即就绪", func(t *testing.T) {
		path := newTask(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), `-NoProfile -NonInteractive -WindowStyle Hidden -Command Start-Sleep -Seconds 60`, true)
		first, err := s.Run(path, "")
		if err != nil || first.State != "running" {
			t.Fatalf("启动: %+v, %v", first, err)
		}
		instanceID := func() string {
			id, err := withTaskService(func(service *ole.IDispatch) (string, error) {
				task, release, err := getRegisteredTask(service, path)
				if err != nil {
					return "", err
				}
				defer release()
				instances, err := oleutil.CallMethod(task, "GetInstances", 0)
				if err != nil {
					return "", err
				}
				defer clearVariant(instances)
				item, err := oleutil.GetProperty(instances.ToIDispatch(), "Item", 1)
				if err != nil {
					return "", err
				}
				defer clearVariant(item)
				return getDispatchString(item.ToIDispatch(), "InstanceGuid"), nil
			})
			if err != nil || id == "" {
				t.Fatalf("实例: %q, %v", id, err)
			}
			return id
		}
		before := instanceID()
		for range 3 {
			restarted, err := s.Restart(path, "")
			if err != nil || restarted.State != "running" {
				t.Fatalf("重启: %+v, %v", restarted, err)
			}
			after := instanceID()
			if before == after {
				t.Fatal("重启没有更换运行实例")
			}
			before = after
		}
		stopped, err := s.Stop(path, "")
		if err != nil || stopped.State != "ready" {
			t.Fatalf("停止: %+v, %v", stopped, err)
		}
	})
	t.Run("短任务正常完成", func(t *testing.T) {
		path := newTask(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), `/c exit 0`, true)
		if _, err := s.Run(path, ""); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := waitScheduledTask(ctx, "等待短任务结束", func() (bool, error) {
			items, err := s.Summaries([]string{path})
			return err == nil && len(items) == 1 && items[0].State == "ready" && items[0].LastResult == 0, err
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("非零退出结果可读取", func(t *testing.T) {
		path := newTask(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), `/c exit 23`, true)
		_, runErr := s.Run(path, "")
		if runErr != nil && !strings.Contains(runErr.Error(), "0x00000017") {
			t.Fatal(runErr)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := waitScheduledTask(ctx, "等待失败任务结束", func() (bool, error) {
			items, err := s.Summaries([]string{path})
			return err == nil && len(items) == 1 && items[0].State == "ready" && items[0].LastResult == 23, err
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("拒绝按需启动", func(t *testing.T) {
		path := newTask(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), `/c exit 0`, false)
		if _, err := s.Run(path, ""); err == nil {
			t.Fatal("应拒绝按需启动")
		}
		if _, err := s.Restart(path, ""); err == nil {
			t.Fatal("应拒绝重启")
		}
	})
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
