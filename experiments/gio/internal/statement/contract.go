// Package statement describes the card checkpoints exposed by the Gio pilot.
package statement

import "github.com/waltwang/nestworth-go/internal/domain"

type Action string

const (
	Start     Action = "start"
	Resume    Action = "resume"
	Check     Action = "check_account"
	Finish    Action = "finish"
	Confirmed        = "confirmed"
)

// Only the Wails whole-account check is exposed here. A prior exception or
// other checkpoint needs the corresponding individual reconciliation tools.
func BatchAvailable(account *domain.ReconciliationAccount) bool {
	if len(account.Items) != 2 {
		return false
	}
	hasScope, hasBalance, pending := false, false, false
	for _, item := range account.Items {
		hasScope = hasScope || item.Kind == "scope"
		hasBalance = hasBalance || (item.Kind == "balance" && item.Value != "")
		if item.Outcome == "confirmed" && !item.RecheckRequired {
			continue
		}
		if item.Outcome != "pending" || item.CheckedAt != "" || item.Note != "" ||
			item.Source != (domain.ReconciliationSource{}) || item.ReferenceValue != "" || item.ReferenceComparable {
			return false
		}
		pending = true
	}
	return hasScope && hasBalance && pending
}
