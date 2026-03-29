//go:build server

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func configureMainWindow(_ *application.App) {
}

func supportsNativeWorkspacePicker() bool {
	return false
}
