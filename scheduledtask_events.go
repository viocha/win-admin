package main

import "github.com/wailsapp/wails/v3/pkg/application"

type ScheduledTaskStateChange struct {
	OperationID string               `json:"operationId"`
	Summary     ScheduledTaskSummary `json:"summary"`
}

func init() {
	application.RegisterEvent[ScheduledTaskStateChange]("scheduled-task:state-changed")
}

func publishScheduledTaskState(operationID string, summary ScheduledTaskSummary) {
	if app := application.Get(); app != nil && operationID != "" {
		app.Event.Emit("scheduled-task:state-changed", ScheduledTaskStateChange{OperationID: operationID, Summary: summary})
	}
}
