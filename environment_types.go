package main

// EnvironmentVariableService 管理 Windows 用户和系统环境变量。
type EnvironmentVariableService struct{}

type EnvironmentVariable struct {
	Name       string `json:"name"`
	Value      string `json:"value"`
	Scope      string `json:"scope"`
	Expandable bool   `json:"expandable"`
}

type SaveEnvironmentVariableRequest struct {
	PreviousName string              `json:"previousName"`
	Variable     EnvironmentVariable `json:"variable"`
}

type EnvironmentPathEntry struct {
	Value     string `json:"value"`
	Expanded  string `json:"expanded"`
	Exists    bool   `json:"exists"`
	Duplicate bool   `json:"duplicate"`
}

type SaveEnvironmentPathRequest struct {
	Scope   string   `json:"scope"`
	Entries []string `json:"entries"`
}
