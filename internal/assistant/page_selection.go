package assistant

import "context"

// PageSelection is a desktop view hint, never a permission or displayed-value
// assertion. The host resolves it against live data through the same read tools.
// No DOM, client metrics, labels, filters, credentials or routes are accepted.
type PageSelection struct {
	Kind      string `json:"kind"`
	AccountID string `json:"accountId"`
}

func (p PageSelection) Valid() bool {
	switch p.Kind {
	case "", "overview", "accounts":
		return p.AccountID == ""
	case "account":
		return canonicalID(p.AccountID)
	default:
		return false
	}
}

type pageSelectionKey struct{}

func SelectedPage(ctx context.Context) PageSelection {
	p, _ := ctx.Value(pageSelectionKey{}).(PageSelection)
	return p
}
