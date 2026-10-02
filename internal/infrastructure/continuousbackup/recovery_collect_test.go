package continuousbackup

import "context"

// Explicit full traversal belongs to integration assertions, never the UI API.
func (m *Manager) RecoveryPoints(ctx context.Context) ([]RecoveryPoint, error) {
	var points []RecoveryPoint
	cursor := ""
	seen := map[string]bool{}
	for {
		page, err := m.RecoveryPointPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		points = append(points, page.Points...)
		if page.NextCursor == "" {
			break
		}
		if seen[page.NextCursor] {
			return nil, ErrUnavailable
		}
		seen[page.NextCursor] = true
		cursor = page.NextCursor
	}
	sortRecoveryPoints(points)
	return points, nil
}
func recoveryPoints(ctx context.Context, b backend) ([]RecoveryPoint, error) {
	var points []RecoveryPoint
	token := ""
	seen := map[string]bool{}
	for {
		page, err := readRecoveryPage(ctx, b, token, nil)
		if err != nil {
			return nil, err
		}
		points = append(points, page.Points...)
		if page.NextCursor == "" {
			break
		}
		if seen[page.NextCursor] {
			return nil, ErrUnavailable
		}
		seen[page.NextCursor] = true
		token = page.NextCursor
	}
	sortRecoveryPoints(points)
	return points, nil
}
