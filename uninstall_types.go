package main

// AppUninstallService 提供 Windows 已安装应用的卸载入口。
type AppUninstallService struct{}

type InstalledApp struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Icon             string `json:"icon"`
	Version          string `json:"version"`
	Publisher        string `json:"publisher"`
	InstallDate      string `json:"installDate"`
	InstallTimestamp int64  `json:"installTimestamp"`
	EstimatedSizeKB  uint64 `json:"estimatedSizeKB"`
	InstallLocation  string `json:"installLocation"`
	RegistryPath     string `json:"registryPath"`
	Scope            string `json:"scope"`
	Architecture     string `json:"architecture"`
}

type InstalledAppSize struct {
	ID              string `json:"id"`
	EstimatedSizeKB uint64 `json:"estimatedSizeKB"`
}
