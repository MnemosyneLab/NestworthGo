//go:build server

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func configureApplicationMenu(*application.App, settings.Language) func(settings.Language) {
	return nil
}
