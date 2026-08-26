// Package app is the small "About" / app-identity service (technical
// design Sec6: "Keep intentionally tiny"). Window-size persistence is
// wired directly in the Wails main.go via app.Window.OnWindowEvent
// (technical design Sec9); it needs no frontend-callable method.
package app

import "github.com/waltwang/nestworth-go/internal/version"

type Service struct{}

func NewService() *Service {
	return &Service{}
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
