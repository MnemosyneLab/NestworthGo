// Package agent exposes local MCP management only to the desktop UI. These
// methods are never registered as MCP tools.
package agent

import (
	"github.com/waltwang/nestworth-go/internal/mcpserver"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type Service struct{ server *mcpserver.Service }

func NewService(server *mcpserver.Service) *Service { return &Service{server: server} }
func (s *Service) Status() mcpserver.Status         { return s.server.Status() }
func (s *Service) Enable(mode string) (mcpserver.Status, error) {
	v, err := s.server.Enable(mode)
	return v, apierror.Wrap(err)
}
func (s *Service) Disable() (mcpserver.Status, error) {
	v, err := s.server.Disable()
	return v, apierror.Wrap(err)
}
func (s *Service) Connection() (mcpserver.Connection, error) {
	v, err := s.server.Connection()
	return v, apierror.Wrap(err)
}
func (s *Service) RecentOperations() ([]mcpserver.OperationSummary, error) {
	v, err := s.server.RecentOperations()
	return v, apierror.Wrap(err)
}
