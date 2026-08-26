// Package app is the small "About" / app-identity service (technical
// design Sec6: "Keep intentionally tiny"). Window-size persistence is
// wired directly in the Wails main.go via app.Window.OnWindowEvent
// (technical design Sec9); it needs no frontend-callable method.
package app

import (
	"github.com/waltwang/nestworth-go/internal/version"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type Service struct {
	// startupErr is the database-open failure from cmd/nestworth, if any.
	// It is stored so Startup() can report it as a DTO instead of leaving
	// the other bound services unregistered (which would reject as a raw
	// Wails "service not found" string rather than a wireError).
	startupErr error
}

func NewService(startupErr error) *Service {
	return &Service{startupErr: startupErr}
}

// AppInfoDTO mirrors internal/version's package-level identity constants.
type AppInfoDTO struct {
	Name        string `json:"name"`
	AppID       string `json:"appId"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Build       string `json:"build"`
}

func (s *Service) AppInfo() AppInfoDTO {
	return AppInfoDTO{Name: version.Name, AppID: version.AppID, Description: version.Description, Version: version.Version, Build: version.Build}
}

// StartupDTO is the blocked-startup contract (implementation plan Phase 5).
// Available is a successful DTO either way so the frontend can render the
// blocked page without calling any other service — those stay unregistered
// when the database could not be opened.
type StartupDTO struct {
	Available bool   `json:"available"`
	Code      string `json:"code,omitempty"`
	Field     string `json:"field,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (s *Service) Startup() StartupDTO {
	if s.startupErr == nil {
		return StartupDTO{Available: true}
	}
	wrapped := apierror.Wrap(s.startupErr)
	wire, ok := wrapped.(*apierror.WireError)
	if !ok || wire == nil {
		return StartupDTO{Available: false, Code: "internal", Message: "an unexpected error occurred"}
	}
	return StartupDTO{Available: false, Code: wire.Code, Field: wire.Field, Message: wire.Message}
}
