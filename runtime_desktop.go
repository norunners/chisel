//go:build !server

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func configureMainWindow(app *application.App) {
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Chisel",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHidden,
		},
		BackgroundColour: application.NewRGB(16, 21, 31),
		URL:              "/",
	})
}

func supportsNativeWorkspacePicker() bool {
	return true
}
