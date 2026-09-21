package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	windowsOptions := application.WindowsOptions{}
	if value := strings.TrimSpace(os.Getenv("WIN_ADMIN_WEBVIEW_DEBUG_PORT")); value != "" {
		port, err := strconv.ParseUint(value, 10, 16)
		if err != nil || port == 0 {
			log.Fatalf("invalid WIN_ADMIN_WEBVIEW_DEBUG_PORT %q", value)
		}
		windowsOptions.AdditionalBrowserArgs = []string{fmt.Sprintf("--remote-debugging-port=%d", port)}
	}

	var window *application.WebviewWindow
	app := application.New(application.Options{
		Name:        "Win Admin",
		Description: "Win Admin 系统管理工具",
		Windows:     windowsOptions,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "win-admin",
			OnSecondInstanceLaunch: func(_ application.SecondInstanceData) {
				if window != nil {
					window.Restore()
					window.Focus()
				}
			},
		},
		Services: []application.Service{
			application.NewService(&ContextMenuService{}),
			application.NewService(&FileAssociationService{}),
			application.NewService(&ScheduledTaskService{}),
			application.NewService(&ScheduledTaskTemplateService{}),
			application.NewService(&EnvironmentVariableService{}),
			application.NewService(&AppUninstallService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Win Admin",
		Width:            1280,
		Height:           820,
		MinWidth:         720,
		MinHeight:        640,
		BackgroundColour: application.NewRGB(250, 250, 250),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
