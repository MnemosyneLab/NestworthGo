// Package mobile exposes connection management only to the local desktop UI.
package mobile

import (
	"github.com/waltwang/nestworth-go/internal/mobileapi"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type Service struct{ server *mobileapi.Server }

func NewService(server *mobileapi.Server) *Service { return &Service{server: server} }
func (s *Service) Status() mobileapi.Status        { return s.server.Status() }
func (s *Service) Configure(config mobileapi.Config) (mobileapi.Status, error) {
	v, err := s.server.Configure(config)
	return v, apierror.Wrap(err)
}
func (s *Service) Pair(name string, permissions mobileapi.Permissions) (mobileapi.Pairing, error) {
	v, err := s.server.Pair(name, permissions)
	return v, apierror.Wrap(err)
}
func (s *Service) Revoke(deviceID string) mobileapi.Status {
	s.server.Revoke(deviceID)
	return s.server.Status()
}
