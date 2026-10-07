//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

type scheduledTaskProcesses []windows.Handle

func (processes scheduledTaskProcesses) close() {
	for _, handle := range processes {
		windows.CloseHandle(handle)
	}
}

func (processes scheduledTaskProcesses) wait(ctx context.Context) error {
	for _, handle := range processes {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("等待任务进程退出失败: %w", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.New("等待任务进程退出需要超时限制")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("等待任务进程退出失败: %w", context.DeadlineExceeded)
		}
		state, err := windows.WaitForSingleObject(handle, uint32((remaining+time.Millisecond-1)/time.Millisecond))
		if err != nil {
			return fmt.Errorf("等待任务进程退出失败: %w", err)
		}
		if state != windows.WAIT_OBJECT_0 {
			return fmt.Errorf("等待任务进程退出失败: %w", context.DeadlineExceeded)
		}
	}
	return nil
}

func captureScheduledTaskProcesses(task *ole.IDispatch) (processes scheduledTaskProcesses, err error) {
	defer func() {
		if err != nil {
			processes.close()
		}
	}()
	instances, err := oleutil.CallMethod(task, "GetInstances", 0)
	if err != nil {
		return nil, schedulerError("读取任务实例失败", err)
	}
	defer clearVariant(instances)
	if instances == nil || instances.ToIDispatch() == nil {
		return nil, errors.New("任务实例不可用")
	}
	count, err := scheduledTaskIntProperty(instances.ToIDispatch(), "Count")
	if err != nil {
		return nil, err
	}
	created := make(map[uint32]int64)
	open := func(pid uint32, parentCreated int64) error {
		if _, ok := created[pid]; ok || pid == 0 {
			return nil
		}
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		} // 进程已退出
		if err != nil {
			return fmt.Errorf("读取任务进程 %d 失败: %w", pid, err)
		}
		var start, exit, kernel, user windows.Filetime
		if err := windows.GetProcessTimes(handle, &start, &exit, &kernel, &user); err != nil {
			windows.CloseHandle(handle)
			return fmt.Errorf("读取任务进程创建时间失败: %w", err)
		}
		if start.Nanoseconds() < parentCreated {
			windows.CloseHandle(handle)
			return nil
		} // 排除复用 PID 的无关进程
		processes = append(processes, handle)
		created[pid] = start.Nanoseconds()
		return nil
	}
	for index := 1; index <= count; index++ {
		item, itemErr := oleutil.GetProperty(instances.ToIDispatch(), "Item", index)
		if itemErr != nil {
			return processes, schedulerError("读取任务实例失败", itemErr)
		}
		if item == nil || item.ToIDispatch() == nil {
			clearVariant(item)
			return processes, errors.New("任务实例不可用")
		}
		pid, pidErr := scheduledTaskIntProperty(item.ToIDispatch(), "EnginePID")
		clearVariant(item)
		if pidErr != nil {
			return processes, pidErr
		}
		if err = open(uint32(pid), 0); err != nil {
			return processes, err
		}
	}
	if len(processes) == 0 {
		return processes, nil
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return processes, fmt.Errorf("读取任务进程树失败: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	var entries []windows.ProcessEntry32
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		entries = append(entries, entry)
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return processes, fmt.Errorf("读取任务进程树失败: %w", err)
	}
	err = nil
	for {
		before := len(processes)
		for _, entry := range entries {
			if parentCreated, ok := created[entry.ParentProcessID]; ok {
				if err = open(entry.ProcessID, parentCreated); err != nil {
					return processes, err
				}
			}
		}
		if len(processes) == before {
			break
		}
	}
	return processes, nil
}

func scheduledTaskProcessAlive(pid uint32) (bool, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取运行进程失败: %w", err)
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	return state == uint32(windows.WAIT_TIMEOUT), err
}
