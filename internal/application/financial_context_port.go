package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"time"
)

// FinancialContextRepository loads and validates scope in the same bounded,
// read-only transaction as its inputs. Historical inputs retain both sides of
// transfers; scope projection happens only after replay.
type FinancialContextRepository interface {
	ReadFinancialContextInputs(context.Context, bool, []domain.AccountID, time.Time) (FinancialContextInputs, error)
}
type FinancialContextInputs = domain.FinancialContextInputs
