package mcpserver

import (
	"context"
	"reflect"
	"sort"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
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
func (s *InProcessSession) cashAccountIdentity(ctx context.Context, command history.ChangeCommandRequest) (*HostCashAccountIdentity, error) {
	if !cashReview(command) {
		return nil, nil
	}
	accounts, err := s.service.app.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	members, err := s.service.app.ListMembers(ctx, true)
	if err != nil {
		return nil, err
	}
	names := map[domain.MemberID]string{}
	for _, member := range members {
		names[member.ID] = strings.Join(strings.Fields(member.Name), " ")
	}
	identities := make([]HostCashAccountIdentity, 0, len(accounts))
	selected := -1
	for _, record := range accounts {
		if record.Account.InstitutionID != nil && strings.TrimSpace(record.InstitutionName) == "" {
			return nil, fail("review_unavailable", "assigned institution identity unavailable")
		}
		owners := []string{}
		for _, share := range record.Ownership.Shares() {
			name := names[share.MemberID]
			if name == "" {
				return nil, fail("review_unavailable", "account owner identity unavailable")
			}
			owners = append(owners, name)
		}
		sort.Strings(owners)
		identity := HostCashAccountIdentity{AccountID: record.Account.ID.String(), Name: strings.Join(strings.Fields(record.Account.Name), " "), Institution: strings.Join(strings.Fields(record.InstitutionName), " "), Owners: owners, AccountType: record.Account.AccountType.String(), DefaultCurrency: record.Account.DefaultCurrency.String()}
		if identity.AccountID == command.AccountID {
			selected = len(identities)
		}
		identities = append(identities, identity)
	}
	if selected < 0 {
		return nil, fail("review_unavailable", "review account unavailable")
	}
	target := identities[selected]
	// Never pretend metadata distinguishes accounts when the rendered identities
	// are identical. Rename an account before using this confirmation path.
	visible := target
	visible.AccountID = ""
	for i, identity := range identities {
		if i == selected {
			continue
		}
		identity.AccountID = ""
		if reflect.DeepEqual(identity, visible) {
			return nil, fail("ambiguous_account_identity", "accounts share the same review identity; give the target a distinct name before proposing")
		}
	}
	return &target, nil
}
