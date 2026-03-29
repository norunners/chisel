package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	settingsService := NewSettingsService()
	permissionService := NewPermissionService()
	acpService := NewACPService(settingsService, permissionService)
	sessionService := NewSessionService(settingsService, acpService, permissionService)

	app := application.New(application.Options{
		Name:        "chisel",
		Description: "ACP-first desktop and browser coding client",
		Flags: map[string]any{
			"supportsNativeWorkspacePicker": supportsNativeWorkspacePicker(),
		},
		Services: []application.Service{
			application.NewService(settingsService),
			application.NewService(permissionService),
			application.NewService(acpService),
			application.NewService(sessionService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Server: application.ServerOptions{
			Host: "localhost",
			Port: 8080,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	configureMainWindow(app)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
