package mcpserver

import (
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

// HostCashAccountIdentity is captured under the same application gate as the
// plan and is part of the review digest. No live labels are joined at display.
// Its internal ID binds the snapshot to the command; desktop DTOs omit that ID.
type HostCashAccountIdentity struct {
	AccountID       string   `json:"accountId"`
	Name            string   `json:"name"`
	Institution     string   `json:"institution"`
	Owners          []string `json:"owners"`
	AccountType     string   `json:"accountType"`
	DefaultCurrency string   `json:"defaultCurrency"`
}

func cashReview(command history.ChangeCommandRequest) bool {
	return (command.Kind == history.ChangeMoneyAdded || command.Kind == history.ChangeMoneyRemoved) && command.HoldingID == ""
}
