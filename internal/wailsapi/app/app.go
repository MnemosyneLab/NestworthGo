// Package app is the small "About" / app-identity service. Window-size
// persistence is wired directly in the Wails main.go via
// app.Window.OnWindowEvent and needs no frontend-callable method.
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
	found      int
	supported  int
	hasSchema  bool
}

func NewService(startupErr error) *Service {
	return &Service{startupErr: startupErr}
}

// NewServiceWithSchema reports a blocked startup together with the schema
// versions that are safe to show in the UI. It never includes a filesystem path.
func NewServiceWithSchema(startupErr error, found, supported int) *Service {
	return &Service{startupErr: startupErr, found: found, supported: supported, hasSchema: true}
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

// StartupDTO is the blocked-startup contract.
// Available is a successful DTO either way so the frontend can render the
// blocked page without calling any other service — those stay unregistered
// when the database could not be opened.
type StartupDTO struct {
	Available              bool   `json:"available"`
	Code                   string `json:"code,omitempty"`
	Field                  string `json:"field,omitempty"`
	Message                string `json:"message,omitempty"`
	FoundSchemaVersion     *int   `json:"foundSchemaVersion,omitempty"`
	SupportedSchemaVersion *int   `json:"supportedSchemaVersion,omitempty"`
}

func (s *Service) Startup() StartupDTO {
	if s == nil || s.startupErr == nil {
		return StartupDTO{Available: true}
	}
	wrapped := apierror.Wrap(s.startupErr)
	wire, ok := wrapped.(*apierror.WireError)
	dto := StartupDTO{Available: false}
	if !ok || wire == nil {
		dto.Code = "internal"
		dto.Message = "an unexpected error occurred"
	} else {
		dto.Code = wire.Code
		dto.Field = wire.Field
		dto.Message = wire.Message
	}
	if s.hasSchema {
		found := s.found
		supported := s.supported
		dto.FoundSchemaVersion = &found
		dto.SupportedSchemaVersion = &supported
	}
	return dto
}
