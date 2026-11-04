package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) ReadAgentAudit(ctx context.Context, f domain.AgentAuditFilter) (domain.AgentAuditSelection, error) {
	h, err := s.requireHousehold(ctx)
	if err != nil {
		return domain.AgentAuditSelection{}, err
	}
	repo, ok := s.repository.(interface {
		ReadAgentAudit(context.Context, domain.HouseholdID, domain.AgentAuditFilter) (domain.AgentAuditSelection, error)
	})
	if !ok {
		return domain.AgentAuditSelection{}, &domain.Error{Code: domain.ErrUnavailable, Message: "filtered Agent audit is unavailable"}
	}
	return repo.ReadAgentAudit(ctx, h.ID, f)
}
