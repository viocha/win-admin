package main

// ScheduledTaskService 提供 Windows Task Scheduler 2.0 的管理能力。
type ScheduledTaskService struct{}

type ScheduledTaskSummary struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Folder      string `json:"folder"`
	Command     string `json:"command"`
	Arguments   string `json:"arguments"`
	Enabled     bool   `json:"enabled"`
	State       string `json:"state"`
	LastRunTime string `json:"lastRunTime"`
	NextRunTime string `json:"nextRunTime"`
	LastResult  int32  `json:"lastResult"`
}

type ScheduledTaskAction struct {
	Command          string `json:"command"`
	Arguments        string `json:"arguments"`
	WorkingDirectory string `json:"workingDirectory"`
}

type ScheduledTaskTrigger struct {
	Type          string   `json:"type"`
	StartBoundary string   `json:"startBoundary"`
	Interval      int      `json:"interval"`
	DaysOfWeek    []string `json:"daysOfWeek"`
	UserID        string   `json:"userId"`
}

type ScheduledTaskSettings struct {
	Compatibility                   int    `json:"compatibility"`
	MultipleInstancesPolicy         string `json:"multipleInstancesPolicy"`
	DisallowStartIfOnBatteries      bool   `json:"disallowStartIfOnBatteries"`
	StopIfGoingOnBatteries          bool   `json:"stopIfGoingOnBatteries"`
	AllowHardTerminate              bool   `json:"allowHardTerminate"`
	StartWhenAvailable              bool   `json:"startWhenAvailable"`
	RunOnlyIfNetworkAvailable       bool   `json:"runOnlyIfNetworkAvailable"`
	StopOnIdleEnd                   bool   `json:"stopOnIdleEnd"`
	RestartOnIdle                   bool   `json:"restartOnIdle"`
	AllowStartOnDemand              bool   `json:"allowStartOnDemand"`
	Hidden                          bool   `json:"hidden"`
	RunOnlyIfIdle                   bool   `json:"runOnlyIfIdle"`
	DisallowStartOnRemoteAppSession bool   `json:"disallowStartOnRemoteAppSession"`
	UseUnifiedSchedulingEngine      bool   `json:"useUnifiedSchedulingEngine"`
	WakeToRun                       bool   `json:"wakeToRun"`
	ExecutionTimeLimit              string `json:"executionTimeLimit"`
	Priority                        int    `json:"priority"`
	RestartInterval                 string `json:"restartInterval"`
	RestartCount                    int    `json:"restartCount"`
}

type ScheduledTaskEdit struct {
	Name        string                `json:"name"`
	Folder      string                `json:"folder"`
	Description string                `json:"description"`
	Enabled     bool                  `json:"enabled"`
	UserID      string                `json:"userId"`
	RunLevel    string                `json:"runLevel"`
	Action      ScheduledTaskAction   `json:"action"`
	Trigger     ScheduledTaskTrigger  `json:"trigger"`
	Settings    ScheduledTaskSettings `json:"settings"`
}

type ScheduledTaskDetails struct {
	Summary        ScheduledTaskSummary `json:"summary"`
	Edit           ScheduledTaskEdit    `json:"edit"`
	Editable       bool                 `json:"editable"`
	ReadOnlyReason string               `json:"readOnlyReason"`
	Managed        bool                 `json:"managed"`
}

type SaveScheduledTaskRequest struct {
	PreviousPath string            `json:"previousPath"`
	Task         ScheduledTaskEdit `json:"task"`
}
