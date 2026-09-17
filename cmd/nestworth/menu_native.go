//go:build !server

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func configureApplicationMenu(app *application.App) {
	app.Menu.SetApplicationMenu(application.DefaultApplicationMenu())
}
