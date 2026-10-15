package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReconciliationRepository is optional for small in-memory application fixtures.
type ReconciliationRepository interface {
	ReconciliationSession(context.Context, string, string) (domain.ReconciliationSession, error)
	ReconciliationSessions(context.Context, string) ([]domain.ReconciliationSession, error)
	ReconciliationRevisions(context.Context) (map[string]int64, error)
	ReconciliationReceipt(context.Context, string, string, string) (*domain.ReconciliationSession, error)
	SaveReconciliation(context.Context, domain.ReconciliationSession, int64, map[string]int64, string, string) error
	ReconciliationFinancialChanges(context.Context, string, int64) ([]domain.ReconciliationFinancialChange, error)
	ReconciliationLinks(context.Context, string, string) ([]domain.ReconciliationLinkedChange, error)
}
