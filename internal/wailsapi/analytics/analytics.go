// Package analytics exposes the current instrument holdings read model.
package analytics

import "github.com/waltwang/nestworth-go/internal/application"

type Service struct{ app *application.Service }

func NewService(app *application.Service) *Service { return &Service{app: app} }
