package application

import (
	"cmp"
	"slices"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Keep the largest path in each class, then name additional paths up to this
// target. Every positive path remains in the detail table.
const overviewNamedAssetPaths = 8

type overviewAssetPathKey struct {
	class, institution, account string
}

type overviewAssetComposition struct {
	result domain.OverviewAssetComposition
	paths  map[overviewAssetPathKey]domain.OverviewAssetPath
}

func (c *overviewAssetComposition) add(amount decimal.Decimal, class string, account domain.Account, institutionNames map[string]string) {
	switch amount.Sign() {
	case -1:
		c.result.Negative = c.result.Negative.Add(amount)
		return
	case 0:
		c.result.ZeroCount++
		return
	}
	var institutionID string
	if account.InstitutionID != nil {
		institutionID = account.InstitutionID.String()
	}
	key := overviewAssetPathKey{class, institutionID, account.ID.String()}
	path, ok := c.paths[key]
	if !ok {
		path = domain.OverviewAssetPath{
			AssetClass: class, InstitutionID: institutionID, InstitutionName: institutionNames[institutionID],
			AccountID: key.account, AccountName: account.Name,
		}
	}
	path.Amount = path.Amount.Add(amount)
	c.paths[key] = path
	c.result.Positive = c.result.Positive.Add(amount)
}

func (c *overviewAssetComposition) finish() domain.OverviewAssetComposition {
	out := c.result
	out.Paths = make([]domain.OverviewAssetPath, 0, len(c.paths))
	for _, path := range c.paths {
		out.Paths = append(out.Paths, path)
	}
	slices.SortFunc(out.Paths, func(a, b domain.OverviewAssetPath) int {
		return cmp.Or(b.Amount.Cmp(a.Amount), cmp.Compare(a.AssetClass, b.AssetClass), cmp.Compare(a.AccountID, b.AccountID))
	})
	if len(out.Paths) == 0 {
		return out
	}

	// An institution is scoped within its class so its outbound widths show
	// that class's allocation to each account, including mixed accounts.
	const rootID = "assets"
	institutionNodeID := func(path domain.OverviewAssetPath) string {
		return rootID + "/" + path.AssetClass + "/institution:" + path.InstitutionID
	}
	namedPaths := make(map[int]bool)
	seenClasses := make(map[string]bool)
	namedInstitutions := make(map[string]bool)
	namePath := func(index int, path domain.OverviewAssetPath) {
		namedPaths[index] = true
		namedInstitutions[institutionNodeID(path)] = true
	}
	for index, path := range out.Paths {
		if !seenClasses[path.AssetClass] {
			seenClasses[path.AssetClass] = true
			namePath(index, path)
		}
	}
	for index, path := range out.Paths {
		if len(namedPaths) >= overviewNamedAssetPaths {
			break
		}
		namePath(index, path)
	}
	out.Nodes = []domain.OverviewAssetNode{{ID: rootID, Kind: "total", Amount: out.Positive}}
	indices := make(map[string]int)
	addNode := func(id, parent, kind, key string, amount decimal.Decimal) {
		if index, ok := indices[id]; ok {
			out.Nodes[index].Amount = out.Nodes[index].Amount.Add(amount)
			return
		}
		indices[id] = len(out.Nodes)
		out.Nodes = append(out.Nodes, domain.OverviewAssetNode{ID: id, ParentID: parent, Kind: kind, Key: key, Amount: amount})
	}
	for index, path := range out.Paths {
		classID := rootID + "/" + path.AssetClass
		institutionID, institutionKind, institutionKey := institutionNodeID(path), "institution", path.InstitutionID
		if !namedInstitutions[institutionID] && path.InstitutionID != "" {
			institutionID, institutionKind, institutionKey = classID+"/other-institutions", "otherInstitutions", ""
		}
		accountID, accountKind, accountKey := institutionID+"/other-accounts", "otherAccounts", ""
		if namedPaths[index] {
			accountID, accountKind, accountKey = institutionID+"/account:"+path.AccountID, "account", path.AccountID
		}
		addNode(classID, rootID, "assetClass", path.AssetClass, path.Amount)
		addNode(institutionID, classID, institutionKind, institutionKey, path.Amount)
		addNode(accountID, institutionID, accountKind, accountKey, path.Amount)
	}
	return out
}
