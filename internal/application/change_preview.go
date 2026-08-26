package application

import "github.com/waltwang/nestworth-go/internal/domain"

// PreviewChange delegates all financial arithmetic to the domain planner. It
// intentionally accepts only injected state and never touches Repository,
// SQLite, UI code, or a market-data provider.
func PreviewChange(state domain.ChangeState, command any) (domain.ChangePreview, error) {
	return domain.PreviewChange(state, command)
}
