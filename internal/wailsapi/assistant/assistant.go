// Package assistant exposes capability status only. There is no generic Wails
// method dispatch, secret input, model call, preview or confirmation endpoint.
package assistant

import "github.com/waltwang/nestworth-go/internal/assistant"

type Service struct{}

func NewService() *Service                { return &Service{} }
func (*Service) Status() assistant.Status { return assistant.Closed("desktop") }
