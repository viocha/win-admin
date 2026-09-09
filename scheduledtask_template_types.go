package main

// ScheduledTaskTemplateService 管理本地计划任务模板。
type ScheduledTaskTemplateService struct{}

type ScheduledTaskTemplate struct {
	ID   string            `json:"id"`
	Name string            `json:"name"`
	Task ScheduledTaskEdit `json:"task"`
}
