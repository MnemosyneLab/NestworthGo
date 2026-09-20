package appports

import (
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
)

// Wire attaches production infrastructure adapters to the application
// composition root. cmd/nestworth and tests call this; application itself
// does not import infrastructure packages.
func Wire(service *application.Service) {
	if service == nil {
		return
	}
	service.SetBackup(backup.NewRuntime())
}
