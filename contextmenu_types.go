package main

// ContextMenuService 提供 Windows Explorer Shell Verb 与上下文菜单扩展的管理能力。
// Windows 具体实现位于 contextmenu_windows.go。
type ContextMenuService struct{}

type MenuEntry struct {
	ID              string      `json:"id"`
	Label           string      `json:"label"`
	DisplayLabel    string      `json:"displayLabel"`
	Target          string      `json:"target"`
	Kind            string      `json:"kind"`
	Command         string      `json:"command"`
	CommandTemplate string      `json:"commandTemplate"`
	Icon            string      `json:"icon"`
	Position        string      `json:"position"`
	Extensions      string      `json:"extensions"`
	ShiftOnly       bool        `json:"shiftOnly"`
	Enabled         bool        `json:"enabled"`
	Managed         bool        `json:"managed"`
	ReadOnly        bool        `json:"readOnly"`
	CanEdit         bool        `json:"canEdit"`
	CanEnable       bool        `json:"canEnable"`
	CanPosition     bool        `json:"canPosition"`
	CanShift        bool        `json:"canShift"`
	CanDelete       bool        `json:"canDelete"`
	RegistryPath    string      `json:"registryPath"`
	Children        []MenuEntry `json:"children"`
}

type SaveMenuRequest struct {
	PreviousTarget string    `json:"previousTarget"`
	PreviousID     string    `json:"previousId"`
	Entry          MenuEntry `json:"entry"`
}

type ContextMenuStatus struct {
	Elevated     bool   `json:"elevated"`
	RegistryRoot string `json:"registryRoot"`
	Scope        string `json:"scope"`
}
