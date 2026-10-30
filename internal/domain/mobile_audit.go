package domain

import "context"

type mobileAuditKey struct{}

// MobileAudit contains identifiers only. Financial facts stay in Activity.
type MobileAudit struct {
	DeviceID    string `json:"deviceId"`
	OperationID string `json:"operationId"`
	RequestID   string `json:"requestId"`
	Kind        string `json:"kind"`
}

func WithMobileAudit(ctx context.Context, a MobileAudit) context.Context {
	return context.WithValue(ctx, mobileAuditKey{}, a)
}
func MobileAuditFrom(ctx context.Context) (MobileAudit, bool) {
	a, ok := ctx.Value(mobileAuditKey{}).(MobileAudit)
	return a, ok
}
