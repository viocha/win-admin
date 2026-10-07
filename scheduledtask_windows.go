//go:build windows

package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	taskEnumHidden             = 1
	taskCreateOrUpdate         = 6
	taskActionExec             = 0
	taskTriggerTime            = 1
	taskTriggerDaily           = 2
	taskTriggerWeekly          = 3
	taskTriggerBoot            = 8
	taskTriggerLogon           = 9
	taskLogonNone              = 0
	taskLogonPassword          = 1
	taskLogonS4U               = 2
	taskLogonInteractiveToken  = 3
	taskLogonGroup             = 4
	taskLogonServiceAccount    = 5
	taskLogonInteractiveOrPass = 6
	taskRunLevelHighest        = 1
	defaultScheduledTaskFolder = `\`
	taskHistoryLog             = "Microsoft-Windows-TaskScheduler/Operational"
	scheduledTaskListWorkers   = 8
)

var weekdayBits = map[string]int{
	"sunday":    1,
	"monday":    2,
	"tuesday":   4,
	"wednesday": 8,
	"thursday":  16,
	"friday":    32,
	"saturday":  64,
}

var orderedWeekdays = []string{
	"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday",
}

func (s *ScheduledTaskService) List() ([]ScheduledTaskSummary, error) {
	folders, err := s.ListFolders()
	if err != nil {
		return nil, err
	}
	folders = append([]string{defaultScheduledTaskFolder}, folders...)

	workerCount := min(scheduledTaskListWorkers, len(folders))
	jobs := make(chan string, len(folders))
	for _, folderPath := range folders {
		jobs <- folderPath
	}
	close(jobs)

	results := make(chan []ScheduledTaskSummary, workerCount)
	errors := make(chan error, workerCount)
	var wg sync.WaitGroup

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, workerErr := withTaskService(func(service *ole.IDispatch) ([]ScheduledTaskSummary, error) {
				items := make([]ScheduledTaskSummary, 0, 64)
				for folderPath := range jobs {
					items = append(items, readScheduledTaskFolder(service, folderPath)...)
				}
				return items, nil
			})
			if workerErr != nil {
				errors <- workerErr
				return
			}
			results <- items
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	items := make([]ScheduledTaskSummary, 0, 128)
	for batch := range results {
		items = append(items, batch...)
	}
	for workerErr := range errors {
		if workerErr != nil {
			return nil, workerErr
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Path) < strings.ToLower(items[j].Path)
	})
	return items, nil
}

func (s *ScheduledTaskService) ListFolders() ([]string, error) {
	return withTaskService(func(service *ole.IDispatch) ([]string, error) {
		rootVariant, err := oleutil.CallMethod(service, "GetFolder", `\`)
		if err != nil {
			return nil, schedulerError("打开计划任务根目录失败", err)
		}
		root := rootVariant.ToIDispatch()
		if root == nil {
			return nil, errors.New("无法访问计划任务根目录")
		}
		defer root.Release()

		paths := make([]string, 0, 64)
		walkScheduledTaskFolders(root, &paths)
		sort.SliceStable(paths, func(i, j int) bool {
			return strings.ToLower(paths[i]) < strings.ToLower(paths[j])
		})
		return paths, nil
	})
}

func (s *ScheduledTaskService) HistoryEnabled() (bool, error) {
	output, err := runTaskHistoryCommand("gl", taskHistoryLog)
	if err != nil {
		return false, fmt.Errorf("读取任务历史状态失败: %w", err)
	}
	match := regexp.MustCompile(`(?mi)^enabled:\s*(true|false)\s*$`).FindStringSubmatch(string(output))
	if len(match) != 2 {
		return false, errors.New("无法识别任务历史状态")
	}
	return strings.EqualFold(match[1], "true"), nil
}

func (s *ScheduledTaskService) SetHistoryEnabled(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	if _, err := runTaskHistoryCommand("sl", taskHistoryLog, "/e:"+value); err != nil {
		return fmt.Errorf("修改任务历史状态失败: %w", err)
	}
	return nil
}

func (s *ScheduledTaskService) History(taskPath string, limit int) ([]ScheduledTaskHistoryEvent, error) {
	taskPath = normalizeTaskPath(taskPath)
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	query := "*[EventData[Data[@Name='TaskName']=" + taskHistoryXPathLiteral(taskPath) + "]]"
	output, err := runTaskHistoryCommand(
		"qe", taskHistoryLog,
		"/q:"+query,
		"/c:"+strconv.Itoa(limit),
		"/rd:true",
		"/f:xml",
	)
	if err != nil {
		return nil, fmt.Errorf("读取任务历史失败: %w", err)
	}
	return parseScheduledTaskHistory(output)
}

func runTaskHistoryCommand(args ...string) ([]byte, error) {
	command := exec.Command("wevtutil.exe", args...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return nil, errors.New(message)
		}
		return nil, err
	}
	return output, nil
}

func taskHistoryXPathLiteral(value string) string {
	if !strings.Contains(value, "'") {
		return "'" + value + "'"
	}
	if !strings.Contains(value, `"`) {
		return `"` + value + `"`
	}
	parts := strings.Split(value, "'")
	quoted := make([]string, 0, len(parts)*2-1)
	for index, part := range parts {
		if index > 0 {
			quoted = append(quoted, `"'"`)
		}
		quoted = append(quoted, "'"+part+"'")
	}
	return "concat(" + strings.Join(quoted, ",") + ")"
}

type scheduledTaskHistoryXML struct {
	Events []struct {
		System struct {
			EventID  int    `xml:"EventID"`
			Level    int    `xml:"Level"`
			RecordID uint64 `xml:"EventRecordID"`
			Created  struct {
				SystemTime string `xml:"SystemTime,attr"`
			} `xml:"TimeCreated"`
		} `xml:"System"`
		EventData struct {
			Name string `xml:"Name,attr"`
			Data []struct {
				Name  string `xml:"Name,attr"`
				Value string `xml:",chardata"`
			} `xml:"Data"`
		} `xml:"EventData"`
	} `xml:"Event"`
}

func parseScheduledTaskHistory(data []byte) ([]ScheduledTaskHistoryEvent, error) {
	if strings.TrimSpace(string(data)) == "" {
		return []ScheduledTaskHistoryEvent{}, nil
	}
	wrapped := append([]byte("<Events>"), data...)
	wrapped = append(wrapped, []byte("</Events>")...)
	var document scheduledTaskHistoryXML
	if err := xml.Unmarshal(wrapped, &document); err != nil {
		return nil, fmt.Errorf("解析任务历史失败: %w", err)
	}

	result := make([]ScheduledTaskHistoryEvent, 0, len(document.Events))
	for _, source := range document.Events {
		values := make(map[string]string, len(source.EventData.Data))
		for _, item := range source.EventData.Data {
			values[item.Name] = strings.TrimSpace(item.Value)
		}
		timestamp := source.System.Created.SystemTime
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			timestamp = parsed.Local().Format(time.RFC3339)
		}
		result = append(result, ScheduledTaskHistoryEvent{
			RecordID:   source.System.RecordID,
			Timestamp:  timestamp,
			EventID:    source.System.EventID,
			Level:      source.System.Level,
			Message:    scheduledTaskHistoryMessage(source.System.EventID, source.EventData.Name),
			ResultCode: scheduledTaskHistoryResultCode(values["ResultCode"]),
			Details:    scheduledTaskHistoryDetails(values),
		})
	}
	return result, nil
}

func scheduledTaskHistoryMessage(eventID int, eventName string) string {
	labels := map[int]string{
		100: "任务已启动",
		101: "任务启动失败",
		102: "任务已完成",
		107: "触发器已触发",
		110: "用户启动任务",
		111: "任务已终止",
		129: "已创建任务进程",
		200: "操作已启动",
		201: "操作已完成",
		202: "操作失败",
	}
	if label := labels[eventID]; label != "" {
		return label
	}
	if eventName != "" {
		return eventName
	}
	return fmt.Sprintf("事件 %d", eventID)
}

func scheduledTaskHistoryResultCode(value string) string {
	if value == "" {
		return ""
	}
	number, err := strconv.ParseUint(value, 0, 32)
	if err != nil {
		return value
	}
	return fmt.Sprintf("0x%08X", uint32(number))
}

func scheduledTaskHistoryDetails(values map[string]string) string {
	keys := []struct {
		name  string
		label string
	}{
		{"UserContext", "用户"},
		{"ActionName", "操作"},
		{"InstanceId", "实例"},
		{"TaskInstanceId", "实例"},
		{"EnginePID", "PID"},
		{"ProcessId", "进程"},
	}
	details := make([]string, 0, len(keys))
	seen := make(map[string]bool)
	for _, key := range keys {
		value := values[key.name]
		if value == "" || seen[key.label+value] {
			continue
		}
		seen[key.label+value] = true
		details = append(details, key.label+": "+value)
	}
	return strings.Join(details, " · ")
}

func (s *ScheduledTaskService) CreateFolder(folderPath string) error {
	if strings.TrimSpace(folderPath) == "" {
		return errors.New("计划任务目录不能为空")
	}
	normalized, err := normalizeTaskFolder(folderPath)
	if err != nil {
		return err
	}
	if normalized == `\` {
		return errors.New("不能创建计划任务根目录")
	}

	_, err = withTaskService(func(service *ole.IDispatch) (struct{}, error) {
		if existingVariant, getErr := oleutil.CallMethod(service, "GetFolder", normalized); getErr == nil {
			if existing := existingVariant.ToIDispatch(); existing != nil {
				existing.Release()
			}
			return struct{}{}, errors.New("同名计划任务文件夹已存在")
		}

		rootVariant, getErr := oleutil.CallMethod(service, "GetFolder", `\`)
		if getErr != nil {
			return struct{}{}, schedulerError("打开计划任务根目录失败", getErr)
		}
		root := rootVariant.ToIDispatch()
		if root == nil {
			return struct{}{}, errors.New("计划任务根目录不可用")
		}
		defer root.Release()

		createdVariant, createErr := oleutil.CallMethod(root, "CreateFolder", strings.TrimPrefix(normalized, `\`), nil)
		if createErr != nil {
			return struct{}{}, schedulerError("创建计划任务文件夹失败", createErr)
		}
		if created := createdVariant.ToIDispatch(); created != nil {
			created.Release()
		}
		return struct{}{}, nil
	})
	return err
}

func (s *ScheduledTaskService) DeleteFolder(folderPath string) error {
	if strings.TrimSpace(folderPath) == "" {
		return errors.New("计划任务目录不能为空")
	}
	normalized, err := normalizeTaskFolder(folderPath)
	if err != nil {
		return err
	}
	if normalized == `\\` {
		return errors.New("不能删除计划任务根目录")
	}

	_, err = withTaskService(func(service *ole.IDispatch) (struct{}, error) {
		if deleteErr := deleteTaskFolderRecursive(service, normalized); deleteErr != nil {
			return struct{}{}, deleteErr
		}
		return struct{}{}, nil
	})
	return err
}

func deleteTaskFolderRecursive(service *ole.IDispatch, folderPath string) error {
	folderVariant, err := oleutil.CallMethod(service, "GetFolder", folderPath)
	if err != nil {
		return schedulerError("打开计划任务文件夹失败", err)
	}
	folder := folderVariant.ToIDispatch()
	if folder == nil {
		return errors.New("计划任务文件夹不可用")
	}

	taskNames := make([]string, 0)
	tasksVariant, tasksErr := oleutil.CallMethod(folder, "GetTasks", taskEnumHidden)
	if tasksErr != nil {
		folder.Release()
		return schedulerError("读取计划任务文件夹任务失败", tasksErr)
	}
	if tasks := tasksVariant.ToIDispatch(); tasks != nil {
		count := getDispatchInt(tasks, "Count")
		for index := 1; index <= count; index++ {
			itemVariant, itemErr := oleutil.GetProperty(tasks, "Item", index)
			if itemErr != nil {
				continue
			}
			task := itemVariant.ToIDispatch()
			if task == nil {
				continue
			}
			if name := getDispatchString(task, "Name"); name != "" {
				taskNames = append(taskNames, name)
			}
			task.Release()
		}
		tasks.Release()
	}

	childPaths := make([]string, 0)
	foldersVariant, foldersErr := oleutil.CallMethod(folder, "GetFolders", 0)
	if foldersErr != nil {
		folder.Release()
		return schedulerError("读取计划任务子文件夹失败", foldersErr)
	}
	if folders := foldersVariant.ToIDispatch(); folders != nil {
		count := getDispatchInt(folders, "Count")
		for index := 1; index <= count; index++ {
			itemVariant, itemErr := oleutil.GetProperty(folders, "Item", index)
			if itemErr != nil {
				continue
			}
			child := itemVariant.ToIDispatch()
			if child == nil {
				continue
			}
			if path := getDispatchString(child, "Path"); path != "" {
				childPaths = append(childPaths, path)
			}
			child.Release()
		}
		folders.Release()
	}

	for _, name := range taskNames {
		result, deleteErr := oleutil.CallMethod(folder, "DeleteTask", name, 0)
		clearVariant(result)
		if deleteErr != nil {
			folder.Release()
			return schedulerError("递归删除计划任务失败", deleteErr)
		}
	}
	folder.Release()

	for _, childPath := range childPaths {
		if err := deleteTaskFolderRecursive(service, childPath); err != nil {
			return err
		}
	}

	parentPath, name := splitTaskPath(folderPath)
	if name == "" {
		return errors.New("计划任务目录无效")
	}
	parentVariant, err := oleutil.CallMethod(service, "GetFolder", parentPath)
	if err != nil {
		return schedulerError("打开计划任务父目录失败", err)
	}
	parent := parentVariant.ToIDispatch()
	if parent == nil {
		return errors.New("计划任务父目录不可用")
	}
	defer parent.Release()
	result, err := oleutil.CallMethod(parent, "DeleteFolder", name, 0)
	clearVariant(result)
	if err != nil {
		return schedulerError("删除计划任务文件夹失败", err)
	}
	return nil
}

func (s *ScheduledTaskService) Get(taskPath string) (ScheduledTaskDetails, error) {
	return withTaskService(func(service *ole.IDispatch) (ScheduledTaskDetails, error) {
		task, release, err := getRegisteredTask(service, taskPath)
		if err != nil {
			return ScheduledTaskDetails{}, err
		}
		defer release()
		return readScheduledTaskDetails(task)
	})
}

func (s *ScheduledTaskService) Save(request SaveScheduledTaskRequest) (ScheduledTaskDetails, error) {
	return withTaskService(func(service *ole.IDispatch) (ScheduledTaskDetails, error) {
		edit, err := validateScheduledTaskEdit(request.Task)
		if err != nil {
			return ScheduledTaskDetails{}, err
		}

		var definition *ole.IDispatch
		var oldPath string
		if strings.TrimSpace(request.PreviousPath) != "" {
			oldPath = normalizeTaskPath(request.PreviousPath)
			oldTask, release, getErr := getRegisteredTask(service, oldPath)
			if getErr != nil {
				return ScheduledTaskDetails{}, getErr
			}
			defer release()

			current, detailsErr := readScheduledTaskDetails(oldTask)
			if detailsErr != nil {
				return ScheduledTaskDetails{}, detailsErr
			}
			if !current.Editable {
				return ScheduledTaskDetails{}, errors.New(current.ReadOnlyReason)
			}

			definitionVariant, propErr := oleutil.GetProperty(oldTask, "Definition")
			if propErr != nil {
				return ScheduledTaskDetails{}, schedulerError("读取任务定义失败", propErr)
			}
			definition = definitionVariant.ToIDispatch()
			if definition == nil {
				return ScheduledTaskDetails{}, errors.New("任务定义不可用")
			}
			defer definition.Release()
		} else {
			definitionVariant, newErr := oleutil.CallMethod(service, "NewTask", 0)
			if newErr != nil {
				return ScheduledTaskDetails{}, schedulerError("创建任务定义失败", newErr)
			}
			definition = definitionVariant.ToIDispatch()
			if definition == nil {
				return ScheduledTaskDetails{}, errors.New("任务定义不可用")
			}
			defer definition.Release()
			if err := configureNewTaskPrincipal(definition); err != nil {
				return ScheduledTaskDetails{}, err
			}
			if err := putRegistrationProperty(definition, "Source", "win-admin"); err != nil {
				return ScheduledTaskDetails{}, err
			}
			if err := configureNewTaskSettings(definition); err != nil {
				return ScheduledTaskDetails{}, err
			}
		}

		if err := applyScheduledTaskEdit(definition, edit); err != nil {
			return ScheduledTaskDetails{}, err
		}

		folder, err := ensureTaskFolder(service, edit.Folder)
		if err != nil {
			return ScheduledTaskDetails{}, err
		}
		defer folder.Release()
		if oldPath == "" {
			exists, existsErr := registeredTaskExistsInFolder(folder, edit.Name)
			if existsErr != nil {
				return ScheduledTaskDetails{}, existsErr
			}
			if exists {
				return ScheduledTaskDetails{}, errors.New("同名计划任务已存在")
			}
		}

		registerUserID, registerPassword, registerLogonType, credentialErr := scheduledTaskRegistrationCredentials(edit, request.Password)
		if credentialErr != nil {
			return ScheduledTaskDetails{}, credentialErr
		}

		registeredVariant, registerErr := oleutil.CallMethod(
			folder,
			"RegisterTaskDefinition",
			edit.Name,
			definition,
			taskCreateOrUpdate,
			registerUserID,
			registerPassword,
			registerLogonType,
			nil,
		)
		if registerErr != nil {
			return ScheduledTaskDetails{}, schedulerError("保存计划任务失败", registerErr)
		}
		if registered := registeredVariant.ToIDispatch(); registered != nil {
			registered.Release()
		}

		newPath := joinTaskPath(edit.Folder, edit.Name)
		if oldPath != "" && !strings.EqualFold(oldPath, newPath) {
			if deleteErr := deleteScheduledTask(service, oldPath); deleteErr != nil {
				_ = deleteScheduledTask(service, newPath)
				return ScheduledTaskDetails{}, fmt.Errorf("移动任务后删除旧任务失败: %w", deleteErr)
			}
		}

		task, release, getErr := getRegisteredTask(service, newPath)
		if getErr != nil {
			return ScheduledTaskDetails{}, getErr
		}
		defer release()
		return readScheduledTaskDetails(task)
	})
}

func (s *ScheduledTaskService) SetEnabled(taskPath string, enabled bool) (ScheduledTaskSummary, error) {
	return withTaskService(func(service *ole.IDispatch) (ScheduledTaskSummary, error) {
		task, release, getErr := getRegisteredTask(service, taskPath)
		if getErr != nil {
			return ScheduledTaskSummary{}, getErr
		}
		defer release()
		result, putErr := oleutil.PutProperty(task, "Enabled", enabled)
		clearVariant(result)
		if putErr != nil {
			return ScheduledTaskSummary{}, schedulerError("修改任务状态失败", putErr)
		}
		return freshScheduledTaskSummary(service, taskPath)
	})
}

// Summaries 批量读取指定任务的实时状态，最多查询 128 个任务。
func (s *ScheduledTaskService) Summaries(taskPaths []string) ([]ScheduledTaskSummary, error) {
	if len(taskPaths) > 128 {
		return nil, errors.New("一次最多查询 128 个计划任务")
	}
	return withTaskService(func(service *ole.IDispatch) ([]ScheduledTaskSummary, error) {
		items := make([]ScheduledTaskSummary, 0, len(taskPaths))
		for _, path := range taskPaths {
			summary, err := freshScheduledTaskSummary(service, path)
			if err != nil {
				return nil, err
			}
			items = append(items, summary)
		}
		return items, nil
	})
}

func (s *ScheduledTaskService) Run(taskPath, operationID string) (ScheduledTaskSummary, error) {
	return controlScheduledTask(taskPath, "run", func(summary ScheduledTaskSummary) { publishScheduledTaskState(operationID, summary) })
}

func (s *ScheduledTaskService) Stop(taskPath, operationID string) (ScheduledTaskSummary, error) {
	return controlScheduledTask(taskPath, "stop", func(summary ScheduledTaskSummary) { publishScheduledTaskState(operationID, summary) })
}

// Restart 确认所有实例停止后启动新实例，并等待启动结果。
func (s *ScheduledTaskService) Restart(taskPath, operationID string) (ScheduledTaskSummary, error) {
	return controlScheduledTask(taskPath, "restart", func(summary ScheduledTaskSummary) { publishScheduledTaskState(operationID, summary) })
}

func controlScheduledTask(taskPath, operation string, onStateChange func(ScheduledTaskSummary)) (ScheduledTaskSummary, error) {
	return withTaskService(func(service *ole.IDispatch) (ScheduledTaskSummary, error) {
		var lastPublished ScheduledTaskSummary
		publish := func(summary ScheduledTaskSummary) {
			if summary != lastPublished {
				lastPublished = summary
				if onStateChange != nil {
					onStateChange(summary)
				}
			}
		}
		task, release, err := getRegisteredTask(service, taskPath)
		if err != nil {
			return ScheduledTaskSummary{}, err
		}
		defer release()
		if operation != "stop" {
			if !getDispatchBool(task, "Enabled") {
				return ScheduledTaskSummary{}, errors.New("计划任务已禁用，无法启动")
			}
			definition, err := oleutil.GetProperty(task, "Definition")
			if err != nil {
				return ScheduledTaskSummary{}, schedulerError("读取任务定义失败", err)
			}
			defer clearVariant(definition)
			if definition == nil || definition.ToIDispatch() == nil {
				return ScheduledTaskSummary{}, errors.New("任务定义不可用")
			}
			settings, err := oleutil.GetProperty(definition.ToIDispatch(), "Settings")
			if err != nil {
				return ScheduledTaskSummary{}, schedulerError("读取任务设置失败", err)
			}
			defer clearVariant(settings)
			if settings == nil || settings.ToIDispatch() == nil {
				return ScheduledTaskSummary{}, errors.New("任务设置不可用")
			}
			if !getDispatchBool(settings.ToIDispatch(), "AllowDemandStart") {
				return ScheduledTaskSummary{}, errors.New("计划任务不允许按需启动")
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if operation != "run" {
			// 在停止前保留进程句柄，等包装器及其后代释放资源后再启动。
			processes, err := captureScheduledTaskProcesses(task)
			if err != nil {
				return ScheduledTaskSummary{}, err
			}
			defer processes.close()
			result, err := oleutil.CallMethod(task, "Stop", 0)
			clearVariant(result)
			if err != nil {
				return ScheduledTaskSummary{}, schedulerError("停止计划任务失败", err)
			}
			err = waitScheduledTask(ctx, "等待计划任务停止失败", func() (bool, error) {
				// 重新获取注册对象，并核对实例数，避免用停止前的快照判断完成。
				current, release, err := getRegisteredTask(service, taskPath)
				if err != nil {
					return false, err
				}
				defer release()
				state, err := scheduledTaskIntProperty(current, "State")
				if err != nil {
					return false, err
				}
				instances, err := oleutil.CallMethod(current, "GetInstances", 0)
				if err != nil {
					return false, schedulerError("读取任务实例失败", err)
				}
				defer clearVariant(instances)
				if instances == nil || instances.ToIDispatch() == nil {
					return false, errors.New("任务实例不可用")
				}
				count, err := scheduledTaskIntProperty(instances.ToIDispatch(), "Count")
				if err != nil {
					return false, err
				}
				summary := readScheduledTaskSummary(current)
				summary.State = scheduledTaskStateName(state)
				publish(summary)
				return count == 0 && (state == 1 || state == 3), err
			})
			if err != nil {
				return ScheduledTaskSummary{}, err
			}
			if err := processes.wait(ctx); err != nil {
				return ScheduledTaskSummary{}, err
			}
			if operation == "stop" {
				return freshScheduledTaskSummary(service, taskPath)
			}
		}
		// 启动使用新的注册对象；保留 Run 返回的实例以确认本次启动。
		current, releaseCurrent, err := getRegisteredTask(service, taskPath)
		if err != nil {
			return ScheduledTaskSummary{}, err
		}
		defer releaseCurrent()
		result, err := oleutil.CallMethod(current, "Run", nil)
		if err != nil {
			clearVariant(result)
			return ScheduledTaskSummary{}, schedulerError("启动计划任务失败", err)
		}
		defer clearVariant(result)
		if result == nil || result.ToIDispatch() == nil {
			return ScheduledTaskSummary{}, errors.New("计划任务未创建运行实例")
		}
		instance := result.ToIDispatch()
		var summary ScheduledTaskSummary
		err = waitScheduledTask(ctx, "等待计划任务启动失败", func() (bool, error) {
			state, stateErr := scheduledTaskIntProperty(instance, "State")
			// 实例已结束时，部分系统返回 SCHED_E_TASK_NOT_RUNNING。
			var oleErr *ole.OleError
			if stateErr != nil && !(errors.As(stateErr, &oleErr) && uint32(oleErr.Code()) == 0x8004130B) {
				return false, stateErr
			}
			var err error
			summary, err = freshScheduledTaskSummary(service, taskPath)
			if err != nil {
				return false, err
			}
			done, err := scheduledTaskStartConfirmed(state, summary)
			if done && state == 4 {
				pid, pidErr := scheduledTaskIntProperty(instance, "EnginePID")
				if pidErr != nil {
					return false, pidErr
				}
				if pid == 0 {
					return false, nil
				}
				alive, pidErr := scheduledTaskProcessAlive(uint32(pid))
				if pidErr != nil {
					return false, pidErr
				}
				if !alive {
					return false, nil
				}
			}
			if summary.State != "running" || done {
				publish(summary)
			}
			return done, err
		})
		return summary, err
	})
}

func scheduledTaskStartConfirmed(instanceState int, summary ScheduledTaskSummary) (bool, error) {
	if instanceState == 2 || summary.State == "queued" {
		return false, nil
	}
	if instanceState == 4 && summary.State == "running" {
		return true, nil
	}
	if instanceState == 4 {
		return false, nil
	}
	if summary.State == "ready" || summary.State == "disabled" {
		switch uint32(summary.LastResult) {
		case 0:
			// 短任务可能在首次查询前完成，按真实状态显示就绪。
			return true, nil
		case 0x41300, 0x41301, 0x41303, 0x41306:
			// 尚未更新本次执行结果，等待调度器完成状态同步。
			return false, nil
		default:
			return false, fmt.Errorf("计划任务执行失败 (0x%08X)", uint32(summary.LastResult))
		}
	}
	return false, nil
}

func waitScheduledTask(ctx context.Context, message string, check func() (bool, error)) error {
	// 状态接口没有变更订阅；仅在操作期间有界查询，不用延时猜测完成。
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: %w", message, err)
		}
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", message, ctx.Err())
		case <-ticker.C:
		}
	}
}

func scheduledTaskIntProperty(task *ole.IDispatch, property string) (int, error) {
	result, err := oleutil.GetProperty(task, property)
	if err != nil {
		return 0, schedulerError("读取任务 "+property+" 失败", err)
	}
	defer clearVariant(result)
	if result != nil {
		switch value := result.Value().(type) {
		case int32:
			return int(value), nil
		case uint32:
			return int(value), nil
		}
	}
	return 0, errors.New("任务 " + property + " 不可用")
}

func freshScheduledTaskSummary(service *ole.IDispatch, path string) (ScheduledTaskSummary, error) {
	task, release, err := getRegisteredTask(service, path)
	if err != nil {
		return ScheduledTaskSummary{}, err
	}
	defer release()
	state, err := scheduledTaskIntProperty(task, "State")
	if err != nil {
		return ScheduledTaskSummary{}, err
	}
	summary := readScheduledTaskSummary(task)
	summary.State = scheduledTaskStateName(state)
	result, err := scheduledTaskIntProperty(task, "LastTaskResult")
	if err != nil {
		return ScheduledTaskSummary{}, err
	}
	summary.LastResult = int32(result)
	return summary, nil
}

func (s *ScheduledTaskService) ExportXML(taskPath string) error {
	type taskXML struct {
		Name string
		XML  string
	}
	value, err := withTaskService(func(service *ole.IDispatch) (taskXML, error) {
		task, release, getErr := getRegisteredTask(service, taskPath)
		if getErr != nil {
			return taskXML{}, getErr
		}
		defer release()
		xml := getDispatchString(task, "Xml")
		if strings.TrimSpace(xml) == "" {
			return taskXML{}, errors.New("计划任务 XML 为空")
		}
		return taskXML{Name: getDispatchString(task, "Name"), XML: xml}, nil
	})
	if err != nil {
		return err
	}

	dialog := application.Get().Dialog.SaveFile()
	dialog.SetOptions(&application.SaveFileDialogOptions{
		CanCreateDirectories: true,
		Title:                "导出计划任务 XML",
		Message:              "导出计划任务 XML",
		Filename:             sanitizeTaskXMLFilename(value.Name) + ".xml",
		Filters: []application.FileFilter{
			{DisplayName: "任务 XML", Pattern: "*.xml"},
		},
	})
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		if isDialogCancelled(err) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if !strings.EqualFold(filepath.Ext(path), ".xml") {
		path += ".xml"
	}
	if err := os.WriteFile(path, []byte(value.XML), 0o644); err != nil {
		return fmt.Errorf("写入任务 XML 失败: %w", err)
	}
	return nil
}

func (s *ScheduledTaskService) ImportXML() (*ScheduledTaskEdit, error) {
	path, err := application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false).
		SetTitle("导入计划任务 XML").
		AddFilter("任务 XML", "*.xml").
		PromptForSingleSelection()
	if err != nil {
		if isDialogCancelled(err) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取任务 XML 失败: %w", err)
	}

	edit, err := withTaskService(func(service *ole.IDispatch) (ScheduledTaskEdit, error) {
		definitionVariant, newErr := oleutil.CallMethod(service, "NewTask", 0)
		if newErr != nil {
			return ScheduledTaskEdit{}, schedulerError("创建任务定义失败", newErr)
		}
		definition := definitionVariant.ToIDispatch()
		if definition == nil {
			return ScheduledTaskEdit{}, errors.New("任务定义不可用")
		}
		defer definition.Release()

		result, putErr := oleutil.PutProperty(definition, "XmlText", string(data))
		clearVariant(result)
		if putErr != nil {
			return ScheduledTaskEdit{}, schedulerError("解析任务 XML 失败", putErr)
		}

		enabled := true
		settingsVariant, settingsErr := oleutil.GetProperty(definition, "Settings")
		if settingsErr == nil {
			settings := settingsVariant.ToIDispatch()
			if settings != nil {
				enabled = getDispatchBool(settings, "Enabled")
				settings.Release()
			}
		}
		summary := ScheduledTaskSummary{Folder: defaultScheduledTaskFolder, Enabled: enabled}
		parsed, editable, reason, _, readErr := readScheduledTaskEdit(definition, summary)
		if readErr != nil {
			return ScheduledTaskEdit{}, readErr
		}
		if !editable {
			if reason == "" {
				reason = "当前 XML 无法映射到结构化编辑器"
			}
			return ScheduledTaskEdit{}, errors.New(reason)
		}
		parsed.Name = ""
		parsed.Folder = defaultScheduledTaskFolder
		return parsed, nil
	})
	if err != nil {
		return nil, err
	}
	return &edit, nil
}

func (s *ScheduledTaskService) Delete(taskPath string) error {
	_, err := withTaskService(func(service *ole.IDispatch) (struct{}, error) {
		return struct{}{}, deleteScheduledTask(service, taskPath)
	})
	return err
}

func (s *ScheduledTaskService) PickExecutable() (string, error) {
	result, err := application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false).
		SetTitle("选择程序").
		AddFilter("可执行文件", "*.exe;*.com;*.cmd;*.bat").
		PromptForSingleSelection()
	if err != nil {
		if isDialogCancelled(err) {
			return "", nil
		}
		return "", err
	}
	return result, nil
}

func sanitizeTaskXMLFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return '_'
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		return "task"
	}
	return name
}

func withTaskService[T any](fn func(*ole.IDispatch) (T, error)) (T, error) {
	var zero T
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return zero, schedulerError("初始化 COM 失败", err)
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return zero, schedulerError("连接 Task Scheduler 失败", err)
	}
	defer unknown.Release()

	service, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return zero, schedulerError("获取 Task Scheduler 接口失败", err)
	}
	defer service.Release()

	connected, err := oleutil.CallMethod(service, "Connect")
	clearVariant(connected)
	if err != nil {
		return zero, schedulerError("连接 Task Scheduler 服务失败", err)
	}
	return fn(service)
}

func readScheduledTaskFolder(service *ole.IDispatch, folderPath string) []ScheduledTaskSummary {
	folderVariant, err := oleutil.CallMethod(service, "GetFolder", folderPath)
	if err != nil {
		return nil
	}
	folder := folderVariant.ToIDispatch()
	if folder == nil {
		return nil
	}
	defer folder.Release()

	items := make([]ScheduledTaskSummary, 0, 16)
	tasksVariant, err := oleutil.CallMethod(folder, "GetTasks", taskEnumHidden)
	if err == nil {
		tasks := tasksVariant.ToIDispatch()
		if tasks != nil {
			count := getDispatchInt(tasks, "Count")
			for index := 1; index <= count; index++ {
				itemVariant, itemErr := oleutil.GetProperty(tasks, "Item", index)
				if itemErr != nil {
					continue
				}
				task := itemVariant.ToIDispatch()
				if task == nil {
					continue
				}
				items = append(items, readScheduledTaskSummary(task))
				task.Release()
			}
			tasks.Release()
		}
	}
	return items
}

func walkScheduledTaskFolders(folder *ole.IDispatch, paths *[]string) {
	foldersVariant, err := oleutil.CallMethod(folder, "GetFolders", 0)
	if err != nil {
		return
	}
	folders := foldersVariant.ToIDispatch()
	if folders == nil {
		return
	}
	defer folders.Release()

	count := getDispatchInt(folders, "Count")
	for index := 1; index <= count; index++ {
		itemVariant, itemErr := oleutil.GetProperty(folders, "Item", index)
		if itemErr != nil {
			continue
		}
		child := itemVariant.ToIDispatch()
		if child == nil {
			continue
		}
		if path := strings.TrimSpace(getDispatchString(child, "Path")); path != "" && path != `\` {
			*paths = append(*paths, path)
		}
		walkScheduledTaskFolders(child, paths)
		child.Release()
	}
}

func readScheduledTaskSummary(task *ole.IDispatch) ScheduledTaskSummary {
	path := getDispatchString(task, "Path")
	folder, _ := splitTaskPath(path)
	command, arguments := readScheduledTaskActionSummary(task)
	return ScheduledTaskSummary{
		Name:        getDispatchString(task, "Name"),
		Path:        path,
		Folder:      folder,
		Command:     command,
		Arguments:   arguments,
		Enabled:     getDispatchBool(task, "Enabled"),
		State:       scheduledTaskStateName(getDispatchInt(task, "State")),
		LastRunTime: getDispatchDate(task, "LastRunTime"),
		NextRunTime: getDispatchDate(task, "NextRunTime"),
		LastResult:  int32(getDispatchInt(task, "LastTaskResult")),
	}
}

func readScheduledTaskActionSummary(task *ole.IDispatch) (string, string) {
	definitionVariant, err := oleutil.GetProperty(task, "Definition")
	if err != nil {
		return "", ""
	}
	definition := definitionVariant.ToIDispatch()
	if definition == nil {
		return "", ""
	}
	defer definition.Release()

	actionsVariant, err := oleutil.GetProperty(definition, "Actions")
	if err != nil {
		return "", ""
	}
	actions := actionsVariant.ToIDispatch()
	if actions == nil {
		return "", ""
	}
	defer actions.Release()

	count := getDispatchInt(actions, "Count")
	for index := 1; index <= count; index++ {
		actionVariant, itemErr := oleutil.GetProperty(actions, "Item", index)
		if itemErr != nil {
			continue
		}
		action := actionVariant.ToIDispatch()
		if action == nil {
			continue
		}
		if getDispatchInt(action, "Type") == taskActionExec {
			command := strings.TrimSpace(getDispatchString(action, "Path"))
			arguments := strings.TrimSpace(getDispatchString(action, "Arguments"))
			action.Release()
			return command, arguments
		}
		action.Release()
	}
	return "", ""
}

func readScheduledTaskDetails(task *ole.IDispatch) (ScheduledTaskDetails, error) {
	summary := readScheduledTaskSummary(task)
	definitionVariant, err := oleutil.GetProperty(task, "Definition")
	if err != nil {
		return ScheduledTaskDetails{}, schedulerError("读取任务定义失败", err)
	}
	definition := definitionVariant.ToIDispatch()
	if definition == nil {
		return ScheduledTaskDetails{}, errors.New("任务定义不可用")
	}
	defer definition.Release()

	edit, editable, reason, managed, err := readScheduledTaskEdit(definition, summary)
	if err != nil {
		return ScheduledTaskDetails{}, err
	}
	return ScheduledTaskDetails{
		Summary:        summary,
		Edit:           edit,
		Editable:       editable,
		ReadOnlyReason: reason,
		Managed:        managed,
	}, nil
}

func readScheduledTaskEdit(definition *ole.IDispatch, summary ScheduledTaskSummary) (ScheduledTaskEdit, bool, string, bool, error) {
	edit := ScheduledTaskEdit{
		Name:     summary.Name,
		Folder:   summary.Folder,
		Enabled:  summary.Enabled,
		RunLevel: "least",
		Trigger: ScheduledTaskTrigger{
			Type:     "manual",
			Interval: 1,
		},
		Settings: defaultScheduledTaskSettings(),
	}

	managed := false
	registrationVariant, err := oleutil.GetProperty(definition, "RegistrationInfo")
	if err == nil {
		registration := registrationVariant.ToIDispatch()
		if registration != nil {
			edit.Description = getDispatchString(registration, "Description")
			managed = strings.EqualFold(getDispatchString(registration, "Source"), "win-admin")
			registration.Release()
		}
	}

	principalVariant, err := oleutil.GetProperty(definition, "Principal")
	if err != nil {
		return edit, false, "无法读取任务运行身份", managed, schedulerError("读取任务运行身份失败", err)
	}
	principal := principalVariant.ToIDispatch()
	if principal == nil {
		return edit, false, "任务运行身份不可用", managed, nil
	}
	logonType := getDispatchInt(principal, "LogonType")
	edit.UserID = getDispatchString(principal, "UserId")
	edit.LogonType = logonType
	if getDispatchInt(principal, "RunLevel") == taskRunLevelHighest {
		edit.RunLevel = "highest"
	}
	principal.Release()
	if logonType == taskLogonGroup || logonType == taskLogonInteractiveOrPass || logonType == taskLogonNone {
		return edit, false, "此任务使用当前编辑器尚未支持的登录方式", managed, nil
	}

	settingsVariant, settingsErr := oleutil.GetProperty(definition, "Settings")
	if settingsErr == nil {
		settings := settingsVariant.ToIDispatch()
		if settings != nil {
			edit.Settings = readScheduledTaskSettings(settings)
			settings.Release()
		}
	}

	actionsVariant, err := oleutil.GetProperty(definition, "Actions")
	if err != nil {
		return edit, false, "无法读取任务操作", managed, schedulerError("读取任务操作失败", err)
	}
	actions := actionsVariant.ToIDispatch()
	if actions == nil {
		return edit, false, "任务操作不可用", managed, nil
	}
	defer actions.Release()
	if getDispatchInt(actions, "Count") != 1 {
		return edit, false, "当前仅支持结构化编辑单个程序操作的任务", managed, nil
	}
	actionVariant, err := oleutil.GetProperty(actions, "Item", 1)
	if err != nil {
		return edit, false, "无法读取任务操作", managed, schedulerError("读取任务操作失败", err)
	}
	action := actionVariant.ToIDispatch()
	if action == nil {
		return edit, false, "任务操作不可用", managed, nil
	}
	if getDispatchInt(action, "Type") != taskActionExec {
		action.Release()
		return edit, false, "当前仅支持结构化编辑启动程序类型的任务", managed, nil
	}
	edit.Action = ScheduledTaskAction{
		Command:          getDispatchString(action, "Path"),
		Arguments:        getDispatchString(action, "Arguments"),
		WorkingDirectory: getDispatchString(action, "WorkingDirectory"),
	}
	action.Release()

	triggersVariant, err := oleutil.GetProperty(definition, "Triggers")
	if err != nil {
		return edit, false, "无法读取任务触发器", managed, schedulerError("读取任务触发器失败", err)
	}
	triggers := triggersVariant.ToIDispatch()
	if triggers == nil {
		return edit, false, "任务触发器不可用", managed, nil
	}
	defer triggers.Release()
	triggerCount := getDispatchInt(triggers, "Count")
	if triggerCount == 0 {
		return edit, true, "", managed, nil
	}
	if triggerCount != 1 {
		return edit, false, "当前仅支持结构化编辑单个触发器的任务", managed, nil
	}

	triggerVariant, err := oleutil.GetProperty(triggers, "Item", 1)
	if err != nil {
		return edit, false, "无法读取任务触发器", managed, schedulerError("读取任务触发器失败", err)
	}
	trigger := triggerVariant.ToIDispatch()
	if trigger == nil {
		return edit, false, "任务触发器不可用", managed, nil
	}
	defer trigger.Release()

	triggerType := getDispatchInt(trigger, "Type")
	switch triggerType {
	case taskTriggerTime:
		edit.Trigger.Type = "once"
		edit.Trigger.StartBoundary = getDispatchString(trigger, "StartBoundary")
	case taskTriggerDaily:
		edit.Trigger.Type = "daily"
		edit.Trigger.StartBoundary = getDispatchString(trigger, "StartBoundary")
		edit.Trigger.Interval = maxInt(1, getDispatchInt(trigger, "DaysInterval"))
	case taskTriggerWeekly:
		edit.Trigger.Type = "weekly"
		edit.Trigger.StartBoundary = getDispatchString(trigger, "StartBoundary")
		edit.Trigger.Interval = maxInt(1, getDispatchInt(trigger, "WeeksInterval"))
		edit.Trigger.DaysOfWeek = weekdaysFromMask(getDispatchInt(trigger, "DaysOfWeek"))
	case taskTriggerBoot:
		edit.Trigger.Type = "startup"
	case taskTriggerLogon:
		edit.Trigger.Type = "logon"
		edit.Trigger.UserID = getDispatchString(trigger, "UserId")
	default:
		return edit, false, "此任务使用当前编辑器尚未支持的触发器", managed, nil
	}
	return edit, true, "", managed, nil
}

func configureNewTaskPrincipal(definition *ole.IDispatch) error {
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("读取当前用户失败: %w", err)
	}
	principalVariant, err := oleutil.GetProperty(definition, "Principal")
	if err != nil {
		return schedulerError("读取任务运行身份失败", err)
	}
	principal := principalVariant.ToIDispatch()
	if principal == nil {
		return errors.New("任务运行身份不可用")
	}
	defer principal.Release()
	if err := putDispatchProperty(principal, "UserId", current.Username); err != nil {
		return schedulerError("设置任务用户失败", err)
	}
	if err := putDispatchProperty(principal, "LogonType", taskLogonInteractiveToken); err != nil {
		return schedulerError("设置任务登录方式失败", err)
	}
	return nil
}

func configureNewTaskSettings(definition *ole.IDispatch) error {
	settingsVariant, err := oleutil.GetProperty(definition, "Settings")
	if err != nil {
		return schedulerError("读取任务设置失败", err)
	}
	settings := settingsVariant.ToIDispatch()
	if settings == nil {
		return errors.New("任务设置不可用")
	}
	defer settings.Release()
	return writeScheduledTaskSettings(settings, defaultScheduledTaskSettings())
}

func putRegistrationProperty(definition *ole.IDispatch, name string, value any) error {
	variant, err := oleutil.GetProperty(definition, "RegistrationInfo")
	if err != nil {
		return schedulerError("读取任务注册信息失败", err)
	}
	registration := variant.ToIDispatch()
	if registration == nil {
		return errors.New("任务注册信息不可用")
	}
	defer registration.Release()
	if err := putDispatchProperty(registration, name, value); err != nil {
		return schedulerError("修改任务注册信息失败", err)
	}
	return nil
}

func applyScheduledTaskEdit(definition *ole.IDispatch, edit ScheduledTaskEdit) error {
	if err := putRegistrationProperty(definition, "Description", edit.Description); err != nil {
		return err
	}

	settingsVariant, err := oleutil.GetProperty(definition, "Settings")
	if err != nil {
		return schedulerError("读取任务设置失败", err)
	}
	settings := settingsVariant.ToIDispatch()
	if settings == nil {
		return errors.New("任务设置不可用")
	}
	if err := putDispatchProperty(settings, "Enabled", edit.Enabled); err != nil {
		settings.Release()
		return schedulerError("修改任务状态失败", err)
	}
	if err := writeScheduledTaskSettings(settings, edit.Settings); err != nil {
		settings.Release()
		return err
	}
	settings.Release()

	principalVariant, err := oleutil.GetProperty(definition, "Principal")
	if err != nil {
		return schedulerError("读取任务运行身份失败", err)
	}
	principal := principalVariant.ToIDispatch()
	if principal == nil {
		return errors.New("任务运行身份不可用")
	}
	runLevel := 0
	if edit.RunLevel == "highest" {
		runLevel = taskRunLevelHighest
	}
	if err := putDispatchProperty(principal, "RunLevel", runLevel); err != nil {
		principal.Release()
		return schedulerError("修改运行级别失败", err)
	}
	if userID := strings.TrimSpace(edit.UserID); userID != "" {
		if err := putDispatchProperty(principal, "UserId", userID); err != nil {
			principal.Release()
			return schedulerError("修改运行账户失败", err)
		}
	}
	if err := putDispatchProperty(principal, "LogonType", edit.LogonType); err != nil {
		principal.Release()
		return schedulerError("修改登录方式失败", err)
	}
	principal.Release()

	actionsVariant, err := oleutil.GetProperty(definition, "Actions")
	if err != nil {
		return schedulerError("读取任务操作失败", err)
	}
	actions := actionsVariant.ToIDispatch()
	if actions == nil {
		return errors.New("任务操作不可用")
	}
	if result, clearErr := oleutil.CallMethod(actions, "Clear"); clearErr != nil {
		actions.Release()
		return schedulerError("清理任务操作失败", clearErr)
	} else {
		clearVariant(result)
	}
	actionVariant, err := oleutil.CallMethod(actions, "Create", taskActionExec)
	if err != nil {
		actions.Release()
		return schedulerError("创建任务操作失败", err)
	}
	action := actionVariant.ToIDispatch()
	if action == nil {
		actions.Release()
		return errors.New("任务操作不可用")
	}
	if err := putDispatchProperty(action, "Path", edit.Action.Command); err != nil {
		action.Release()
		actions.Release()
		return schedulerError("设置程序路径失败", err)
	}
	if err := putDispatchProperty(action, "Arguments", edit.Action.Arguments); err != nil {
		action.Release()
		actions.Release()
		return schedulerError("设置程序参数失败", err)
	}
	if err := putDispatchProperty(action, "WorkingDirectory", edit.Action.WorkingDirectory); err != nil {
		action.Release()
		actions.Release()
		return schedulerError("设置工作目录失败", err)
	}
	action.Release()
	actions.Release()

	triggersVariant, err := oleutil.GetProperty(definition, "Triggers")
	if err != nil {
		return schedulerError("读取任务触发器失败", err)
	}
	triggers := triggersVariant.ToIDispatch()
	if triggers == nil {
		return errors.New("任务触发器不可用")
	}
	defer triggers.Release()
	if result, clearErr := oleutil.CallMethod(triggers, "Clear"); clearErr != nil {
		return schedulerError("清理任务触发器失败", clearErr)
	} else {
		clearVariant(result)
	}
	if edit.Trigger.Type == "manual" {
		return nil
	}

	triggerType := 0
	switch edit.Trigger.Type {
	case "once":
		triggerType = taskTriggerTime
	case "daily":
		triggerType = taskTriggerDaily
	case "weekly":
		triggerType = taskTriggerWeekly
	case "startup":
		triggerType = taskTriggerBoot
	case "logon":
		triggerType = taskTriggerLogon
	default:
		return errors.New("不支持的触发器类型")
	}

	triggerVariant, err := oleutil.CallMethod(triggers, "Create", triggerType)
	if err != nil {
		return schedulerError("创建任务触发器失败", err)
	}
	trigger := triggerVariant.ToIDispatch()
	if trigger == nil {
		return errors.New("任务触发器不可用")
	}
	defer trigger.Release()
	if err := putDispatchProperty(trigger, "Enabled", true); err != nil {
		return schedulerError("启用任务触发器失败", err)
	}

	switch edit.Trigger.Type {
	case "once", "daily", "weekly":
		if err := putDispatchProperty(trigger, "StartBoundary", edit.Trigger.StartBoundary); err != nil {
			return schedulerError("设置触发时间失败", err)
		}
	}
	if edit.Trigger.Type == "daily" {
		if err := putDispatchProperty(trigger, "DaysInterval", edit.Trigger.Interval); err != nil {
			return schedulerError("设置每日间隔失败", err)
		}
	}
	if edit.Trigger.Type == "weekly" {
		if err := putDispatchProperty(trigger, "WeeksInterval", edit.Trigger.Interval); err != nil {
			return schedulerError("设置每周间隔失败", err)
		}
		if err := putDispatchProperty(trigger, "DaysOfWeek", weekdayMask(edit.Trigger.DaysOfWeek)); err != nil {
			return schedulerError("设置星期失败", err)
		}
	}
	if edit.Trigger.Type == "logon" {
		if err := putDispatchProperty(trigger, "UserId", strings.TrimSpace(edit.Trigger.UserID)); err != nil {
			return schedulerError("设置登录用户失败", err)
		}
	}
	return nil
}

func validateScheduledTaskEdit(edit ScheduledTaskEdit) (ScheduledTaskEdit, error) {
	edit.Name = strings.TrimSpace(edit.Name)
	if edit.Name == "" {
		return edit, errors.New("任务名称不能为空")
	}
	if strings.ContainsAny(edit.Name, `\/`) || edit.Name == "." || edit.Name == ".." {
		return edit, errors.New("任务名称包含无效字符")
	}
	folder, err := normalizeTaskFolder(edit.Folder)
	if err != nil {
		return edit, err
	}
	edit.Folder = folder
	edit.Action.Command = resolveExecutablePath(edit.Action.Command)
	if edit.Action.Command == "" {
		return edit, errors.New("程序路径不能为空")
	}
	if edit.RunLevel != "highest" {
		edit.RunLevel = "least"
	}
	if edit.LogonType == taskLogonNone {
		edit.LogonType = taskLogonInteractiveToken
	}
	switch edit.LogonType {
	case taskLogonPassword, taskLogonS4U, taskLogonInteractiveToken, taskLogonServiceAccount:
	default:
		return edit, errors.New("任务登录方式无效")
	}
	if edit.Trigger.Type == "" {
		edit.Trigger.Type = "manual"
	}

	switch edit.Trigger.Type {
	case "manual", "startup", "logon":
		edit.Trigger.StartBoundary = ""
		edit.Trigger.Interval = 1
	case "once":
		boundary, boundaryErr := normalizeStartBoundary(edit.Trigger.StartBoundary)
		if boundaryErr != nil {
			return edit, boundaryErr
		}
		edit.Trigger.StartBoundary = boundary
		edit.Trigger.Interval = 1
	case "daily":
		boundary, boundaryErr := normalizeStartBoundary(edit.Trigger.StartBoundary)
		if boundaryErr != nil {
			return edit, boundaryErr
		}
		edit.Trigger.StartBoundary = boundary
		if edit.Trigger.Interval < 1 || edit.Trigger.Interval > 365 {
			return edit, errors.New("每日间隔必须在 1 到 365 天之间")
		}
	case "weekly":
		boundary, boundaryErr := normalizeStartBoundary(edit.Trigger.StartBoundary)
		if boundaryErr != nil {
			return edit, boundaryErr
		}
		edit.Trigger.StartBoundary = boundary
		if edit.Trigger.Interval < 1 || edit.Trigger.Interval > 52 {
			return edit, errors.New("每周间隔必须在 1 到 52 周之间")
		}
		if weekdayMask(edit.Trigger.DaysOfWeek) == 0 {
			return edit, errors.New("每周任务至少选择一天")
		}
	default:
		return edit, errors.New("不支持的触发器类型")
	}

	settings, settingsErr := normalizeScheduledTaskSettings(edit.Settings)
	if settingsErr != nil {
		return edit, settingsErr
	}
	edit.Settings = settings
	return edit, nil
}

func scheduledTaskRegistrationCredentials(edit ScheduledTaskEdit, password string) (any, any, int, error) {
	switch edit.LogonType {
	case taskLogonInteractiveToken:
		return nil, nil, taskLogonNone, nil
	case taskLogonPassword:
		userID, err := scheduledTaskUserID(edit.UserID)
		if err != nil {
			return nil, nil, 0, err
		}
		if password == "" {
			return nil, nil, 0, errors.New("不管用户是否登录都要运行时需要输入账户密码")
		}
		return userID, password, taskLogonPassword, nil
	case taskLogonS4U:
		userID, err := scheduledTaskUserID(edit.UserID)
		if err != nil {
			return nil, nil, 0, err
		}
		return userID, nil, taskLogonS4U, nil
	case taskLogonServiceAccount:
		userID := strings.TrimSpace(edit.UserID)
		if userID == "" {
			return nil, nil, 0, errors.New("服务账户任务缺少运行账户")
		}
		return userID, nil, taskLogonServiceAccount, nil
	default:
		return nil, nil, 0, errors.New("任务登录方式无效")
	}
}

func scheduledTaskUserID(value string) (string, error) {
	if userID := strings.TrimSpace(value); userID != "" {
		return userID, nil
	}
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("读取当前用户失败: %w", err)
	}
	return current.Username, nil
}

func defaultScheduledTaskSettings() ScheduledTaskSettings {
	return ScheduledTaskSettings{
		Compatibility:           4,
		MultipleInstancesPolicy: "ignoreNew",
		AllowHardTerminate:      true,
		StartWhenAvailable:      true,
		AllowStartOnDemand:      true,
		IdleDuration:            "PT10M",
		IdleWaitTimeout:         "PT1H",
		ExecutionTimeLimit:      "PT0S",
		DeleteExpiredTaskAfter:  "",
		Priority:                7,
		RestartInterval:         "PT1M",
		RestartCount:            3,
	}
}

func readScheduledTaskSettings(settings *ole.IDispatch) ScheduledTaskSettings {
	result := defaultScheduledTaskSettings()
	result.Compatibility = getDispatchInt(settings, "Compatibility")
	result.MultipleInstancesPolicy = multipleInstancesPolicyName(getDispatchInt(settings, "MultipleInstances"))
	result.DisallowStartIfOnBatteries = getDispatchBool(settings, "DisallowStartIfOnBatteries")
	result.StopIfGoingOnBatteries = getDispatchBool(settings, "StopIfGoingOnBatteries")
	result.AllowHardTerminate = getDispatchBool(settings, "AllowHardTerminate")
	result.StartWhenAvailable = getDispatchBool(settings, "StartWhenAvailable")
	result.RunOnlyIfNetworkAvailable = getDispatchBool(settings, "RunOnlyIfNetworkAvailable")
	result.AllowStartOnDemand = getDispatchBool(settings, "AllowDemandStart")
	result.Hidden = getDispatchBool(settings, "Hidden")
	result.RunOnlyIfIdle = getDispatchBool(settings, "RunOnlyIfIdle")
	result.DisallowStartOnRemoteAppSession = getDispatchBool(settings, "DisallowStartOnRemoteAppSession")
	result.UseUnifiedSchedulingEngine = getDispatchBool(settings, "UseUnifiedSchedulingEngine")
	result.WakeToRun = getDispatchBool(settings, "WakeToRun")
	result.ExecutionTimeLimit = getDispatchString(settings, "ExecutionTimeLimit")
	result.DeleteExpiredTaskAfter = getDispatchString(settings, "DeleteExpiredTaskAfter")
	result.Priority = getDispatchInt(settings, "Priority")

	idleVariant, idleErr := oleutil.GetProperty(settings, "IdleSettings")
	if idleErr == nil {
		idle := idleVariant.ToIDispatch()
		if idle != nil {
			if idleDuration := getDispatchString(idle, "IdleDuration"); idleDuration != "" {
				result.IdleDuration = idleDuration
			}
			if waitTimeout := getDispatchString(idle, "WaitTimeout"); waitTimeout != "" {
				result.IdleWaitTimeout = waitTimeout
			}
			result.StopOnIdleEnd = getDispatchBool(idle, "StopOnIdleEnd")
			result.RestartOnIdle = getDispatchBool(idle, "RestartOnIdle")
			idle.Release()
		}
	}

	if restartInterval := getDispatchString(settings, "RestartInterval"); restartInterval != "" {
		result.RestartInterval = restartInterval
	}
	result.RestartCount = getDispatchInt(settings, "RestartCount")
	return result
}

func writeScheduledTaskSettings(settings *ole.IDispatch, value ScheduledTaskSettings) error {
	value, err := normalizeScheduledTaskSettings(value)
	if err != nil {
		return err
	}

	properties := []struct {
		name  string
		value any
	}{
		{"Compatibility", value.Compatibility},
		{"MultipleInstances", multipleInstancesPolicyValue(value.MultipleInstancesPolicy)},
		{"DisallowStartIfOnBatteries", value.DisallowStartIfOnBatteries},
		{"StopIfGoingOnBatteries", value.StopIfGoingOnBatteries},
		{"AllowHardTerminate", value.AllowHardTerminate},
		{"StartWhenAvailable", value.StartWhenAvailable},
		{"RunOnlyIfNetworkAvailable", value.RunOnlyIfNetworkAvailable},
		{"AllowDemandStart", value.AllowStartOnDemand},
		{"Hidden", value.Hidden},
		{"RunOnlyIfIdle", value.RunOnlyIfIdle},
		{"DisallowStartOnRemoteAppSession", value.DisallowStartOnRemoteAppSession},
		{"UseUnifiedSchedulingEngine", value.UseUnifiedSchedulingEngine},
		{"WakeToRun", value.WakeToRun},
		{"ExecutionTimeLimit", value.ExecutionTimeLimit},
		{"DeleteExpiredTaskAfter", value.DeleteExpiredTaskAfter},
		{"Priority", value.Priority},
	}
	for _, property := range properties {
		if err := putDispatchProperty(settings, property.name, property.value); err != nil {
			return schedulerError("修改任务设置失败", err)
		}
	}

	if value.RestartCount > 0 {
		if err := putDispatchProperty(settings, "RestartInterval", value.RestartInterval); err != nil {
			return schedulerError("修改失败重启设置失败", err)
		}
		if err := putDispatchProperty(settings, "RestartCount", value.RestartCount); err != nil {
			return schedulerError("修改失败重启设置失败", err)
		}
	} else {
		if err := putDispatchProperty(settings, "RestartInterval", ""); err != nil {
			return schedulerError("修改失败重启设置失败", err)
		}
		if err := putDispatchProperty(settings, "RestartCount", 0); err != nil {
			return schedulerError("修改失败重启设置失败", err)
		}
	}

	idleVariant, err := oleutil.GetProperty(settings, "IdleSettings")
	if err != nil {
		return schedulerError("读取空闲设置失败", err)
	}
	idle := idleVariant.ToIDispatch()
	if idle == nil {
		return errors.New("任务空闲设置不可用")
	}
	defer idle.Release()
	if err := putDispatchProperty(idle, "IdleDuration", value.IdleDuration); err != nil {
		return schedulerError("修改空闲持续时间失败", err)
	}
	if err := putDispatchProperty(idle, "WaitTimeout", value.IdleWaitTimeout); err != nil {
		return schedulerError("修改空闲等待时间失败", err)
	}
	if err := putDispatchProperty(idle, "StopOnIdleEnd", value.StopOnIdleEnd); err != nil {
		return schedulerError("修改空闲设置失败", err)
	}
	if err := putDispatchProperty(idle, "RestartOnIdle", value.RestartOnIdle); err != nil {
		return schedulerError("修改空闲设置失败", err)
	}
	return nil
}

func normalizeScheduledTaskSettings(value ScheduledTaskSettings) (ScheduledTaskSettings, error) {
	if value.Compatibility < 0 || value.Compatibility > 6 {
		return value, errors.New("任务兼容级别无效")
	}
	if value.MultipleInstancesPolicy == "" {
		value.MultipleInstancesPolicy = "ignoreNew"
	}
	switch value.MultipleInstancesPolicy {
	case "parallel", "queue", "ignoreNew", "stopExisting":
	default:
		return value, errors.New("多实例策略无效")
	}
	value.IdleDuration = strings.TrimSpace(value.IdleDuration)
	if value.IdleDuration == "" {
		value.IdleDuration = "PT10M"
	}
	idleDurationSeconds, ok := parseTaskSchedulerDurationSeconds(value.IdleDuration)
	if !ok || idleDurationSeconds < 60 {
		return value, errors.New("空闲持续时间必须至少为 1 分钟")
	}
	value.IdleWaitTimeout = strings.TrimSpace(value.IdleWaitTimeout)
	if value.IdleWaitTimeout == "" {
		value.IdleWaitTimeout = "PT1H"
	}
	idleWaitSeconds, ok := parseTaskSchedulerDurationSeconds(value.IdleWaitTimeout)
	if !ok || idleWaitSeconds < 60 {
		return value, errors.New("等待空闲时间必须至少为 1 分钟")
	}
	value.ExecutionTimeLimit = strings.TrimSpace(value.ExecutionTimeLimit)
	if value.ExecutionTimeLimit == "" {
		value.ExecutionTimeLimit = "PT0S"
	}
	value.DeleteExpiredTaskAfter = strings.TrimSpace(value.DeleteExpiredTaskAfter)
	if value.DeleteExpiredTaskAfter != "" {
		deleteAfterSeconds, ok := parseTaskSchedulerDurationSeconds(value.DeleteExpiredTaskAfter)
		if !ok || deleteAfterSeconds <= 0 {
			return value, errors.New("无后续计划删除等待时间无效")
		}
	}
	value.RestartInterval = strings.TrimSpace(value.RestartInterval)
	if value.RestartInterval == "" {
		value.RestartInterval = "PT1M"
	}
	if value.Priority < 0 || value.Priority > 10 {
		return value, errors.New("优先级必须在 0 到 10 之间")
	}
	if value.RestartCount < 0 || value.RestartCount > 255 {
		return value, errors.New("失败重启次数必须在 0 到 255 之间")
	}
	if value.RestartCount > 0 {
		restartSeconds, ok := parseTaskSchedulerDurationSeconds(value.RestartInterval)
		if !ok {
			return value, errors.New("失败重启间隔格式无效")
		}
		if restartSeconds < 60 || restartSeconds > 31*24*60*60 {
			return value, errors.New("失败重启间隔必须在 1 分钟到 31 天之间")
		}
	}
	return value, nil
}

var taskSchedulerDurationPattern = regexp.MustCompile(`(?i)^P(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

func parseTaskSchedulerDurationSeconds(value string) (float64, bool) {
	matches := taskSchedulerDurationPattern.FindStringSubmatch(strings.TrimSpace(value))
	if matches == nil {
		return 0, false
	}

	hasComponent := false
	total := 0.0
	multipliers := []float64{24 * 60 * 60, 60 * 60, 60, 1}
	for index, multiplier := range multipliers {
		part := matches[index+1]
		if part == "" {
			continue
		}
		hasComponent = true
		amount, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, false
		}
		total += amount * multiplier
	}
	return total, hasComponent
}

func multipleInstancesPolicyName(value int) string {
	switch value {
	case 0:
		return "parallel"
	case 1:
		return "queue"
	case 3:
		return "stopExisting"
	default:
		return "ignoreNew"
	}
}

func multipleInstancesPolicyValue(value string) int {
	switch value {
	case "parallel":
		return 0
	case "queue":
		return 1
	case "stopExisting":
		return 3
	default:
		return 2
	}
}

func normalizeStartBoundary(value string) (string, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed.Format("2006-01-02T15:04:05"), nil
		}
	}
	return "", errors.New("触发时间格式无效")
}

func ensureTaskFolder(service *ole.IDispatch, raw string) (*ole.IDispatch, error) {
	folderPath, err := normalizeTaskFolder(raw)
	if err != nil {
		return nil, err
	}
	rootVariant, err := oleutil.CallMethod(service, "GetFolder", `\`)
	if err != nil {
		return nil, schedulerError("打开计划任务根目录失败", err)
	}
	current := rootVariant.ToIDispatch()
	if current == nil {
		return nil, errors.New("计划任务根目录不可用")
	}
	if folderPath == `\` {
		return current, nil
	}

	currentPath := ""
	for _, segment := range strings.Split(strings.TrimPrefix(folderPath, `\`), `\`) {
		if segment == "" {
			continue
		}
		currentPath += `\` + segment
		nextVariant, getErr := oleutil.CallMethod(service, "GetFolder", currentPath)
		var next *ole.IDispatch
		if getErr == nil {
			next = nextVariant.ToIDispatch()
		} else {
			createdVariant, createErr := oleutil.CallMethod(current, "CreateFolder", segment, nil)
			if createErr != nil {
				current.Release()
				return nil, schedulerError("创建计划任务目录失败", createErr)
			}
			next = createdVariant.ToIDispatch()
		}
		if next == nil {
			current.Release()
			return nil, errors.New("计划任务目录不可用")
		}
		current.Release()
		current = next
	}
	return current, nil
}

func getRegisteredTask(service *ole.IDispatch, rawPath string) (*ole.IDispatch, func(), error) {
	folderPath, name := splitTaskPath(rawPath)
	if name == "" {
		return nil, func() {}, errors.New("计划任务路径无效")
	}
	folderVariant, err := oleutil.CallMethod(service, "GetFolder", folderPath)
	if err != nil {
		return nil, func() {}, schedulerError("打开计划任务目录失败", err)
	}
	folder := folderVariant.ToIDispatch()
	if folder == nil {
		return nil, func() {}, errors.New("计划任务目录不可用")
	}
	taskVariant, err := oleutil.CallMethod(folder, "GetTask", name)
	if err != nil {
		folder.Release()
		return nil, func() {}, schedulerError("读取计划任务失败", err)
	}
	task := taskVariant.ToIDispatch()
	if task == nil {
		folder.Release()
		return nil, func() {}, errors.New("计划任务不可用")
	}
	return task, func() {
		task.Release()
		folder.Release()
	}, nil
}

func registeredTaskExistsInFolder(folder *ole.IDispatch, name string) (bool, error) {
	tasksVariant, err := oleutil.CallMethod(folder, "GetTasks", taskEnumHidden)
	if err != nil {
		return false, schedulerError("读取计划任务目录失败", err)
	}
	tasks := tasksVariant.ToIDispatch()
	if tasks == nil {
		return false, errors.New("计划任务列表不可用")
	}
	defer tasks.Release()

	count := getDispatchInt(tasks, "Count")
	for index := 1; index <= count; index++ {
		itemVariant, itemErr := oleutil.GetProperty(tasks, "Item", index)
		if itemErr != nil {
			continue
		}
		task := itemVariant.ToIDispatch()
		if task == nil {
			continue
		}
		currentName := getDispatchString(task, "Name")
		task.Release()
		if strings.EqualFold(currentName, name) {
			return true, nil
		}
	}
	return false, nil
}

func deleteScheduledTask(service *ole.IDispatch, rawPath string) error {
	folderPath, name := splitTaskPath(rawPath)
	if name == "" {
		return errors.New("计划任务路径无效")
	}
	folderVariant, err := oleutil.CallMethod(service, "GetFolder", folderPath)
	if err != nil {
		return schedulerError("打开计划任务目录失败", err)
	}
	folder := folderVariant.ToIDispatch()
	if folder == nil {
		return errors.New("计划任务目录不可用")
	}
	defer folder.Release()
	result, err := oleutil.CallMethod(folder, "DeleteTask", name, 0)
	clearVariant(result)
	if err != nil {
		return schedulerError("删除计划任务失败", err)
	}
	return nil
}

func normalizeTaskFolder(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "/", `\`))
	if raw == "" {
		raw = defaultScheduledTaskFolder
	}
	if !strings.HasPrefix(raw, `\`) {
		raw = `\` + raw
	}
	for strings.Contains(raw, `\\`) {
		raw = strings.ReplaceAll(raw, `\\`, `\`)
	}
	if len(raw) > 1 {
		raw = strings.TrimSuffix(raw, `\`)
	}
	for _, segment := range strings.Split(strings.TrimPrefix(raw, `\`), `\`) {
		if segment == "." || segment == ".." {
			return "", errors.New("计划任务目录无效")
		}
	}
	return raw, nil
}

func normalizeTaskPath(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "/", `\`))
	if !strings.HasPrefix(raw, `\`) {
		raw = `\` + raw
	}
	for strings.Contains(raw, `\\`) {
		raw = strings.ReplaceAll(raw, `\\`, `\`)
	}
	return raw
}

func splitTaskPath(raw string) (string, string) {
	path := normalizeTaskPath(raw)
	index := strings.LastIndex(path, `\`)
	if index < 0 || index == len(path)-1 {
		return `\`, ""
	}
	folder := path[:index]
	if folder == "" {
		folder = `\`
	}
	return folder, path[index+1:]
}

func joinTaskPath(folder, name string) string {
	folder, _ = normalizeTaskFolder(folder)
	if folder == `\` {
		return `\` + name
	}
	return folder + `\` + name
}

func weekdayMask(days []string) int {
	mask := 0
	for _, day := range days {
		mask |= weekdayBits[strings.ToLower(strings.TrimSpace(day))]
	}
	return mask
}

func weekdaysFromMask(mask int) []string {
	result := make([]string, 0, 7)
	for _, day := range orderedWeekdays {
		if mask&weekdayBits[day] != 0 {
			result = append(result, day)
		}
	}
	return result
}

func scheduledTaskStateName(state int) string {
	switch state {
	case 1:
		return "disabled"
	case 2:
		return "queued"
	case 3:
		return "ready"
	case 4:
		return "running"
	default:
		return "unknown"
	}
}

func getDispatchString(dispatch *ole.IDispatch, name string) string {
	variant, err := oleutil.GetProperty(dispatch, name)
	if err != nil || variant == nil {
		return ""
	}
	defer variant.Clear()
	if value, ok := variant.Value().(string); ok {
		return value
	}
	return variant.ToString()
}

func getDispatchInt(dispatch *ole.IDispatch, name string) int {
	variant, err := oleutil.GetProperty(dispatch, name)
	if err != nil || variant == nil {
		return 0
	}
	defer variant.Clear()
	switch value := variant.Value().(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	default:
		return 0
	}
}

func getDispatchBool(dispatch *ole.IDispatch, name string) bool {
	variant, err := oleutil.GetProperty(dispatch, name)
	if err != nil || variant == nil {
		return false
	}
	defer variant.Clear()
	value, _ := variant.Value().(bool)
	return value
}

func getDispatchDate(dispatch *ole.IDispatch, name string) string {
	variant, err := oleutil.GetProperty(dispatch, name)
	if err != nil || variant == nil {
		return ""
	}
	defer variant.Clear()
	value, ok := variant.Value().(time.Time)
	if !ok || value.Year() < 2001 {
		return ""
	}
	return value.Format("2006-01-02T15:04:05")
}

func putDispatchProperty(dispatch *ole.IDispatch, name string, value any) error {
	result, err := oleutil.PutProperty(dispatch, name, value)
	clearVariant(result)
	return err
}

func clearVariant(variant *ole.VARIANT) {
	if variant != nil {
		_ = variant.Clear()
	}
}

func schedulerError(action string, err error) error {
	if err == nil {
		return nil
	}
	var oleErr *ole.OleError
	if errors.As(err, &oleErr) {
		return fmt.Errorf("%s: %w (0x%08X)", action, oleErr, uint32(oleErr.Code()))
	}
	return fmt.Errorf("%s: %w", action, err)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
