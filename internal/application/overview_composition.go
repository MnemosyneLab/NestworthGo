package application

import (
	"cmp"
	"slices"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

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
	if len(out.Paths) == 0 {
		return out
	}

	// Keep siblings contiguous at each depth so the chart can lay out the
	// tree in this order without crossing paths from different branches.
	slices.SortFunc(out.Paths, func(a, b domain.OverviewAssetPath) int {
		return cmp.Or(cmp.Compare(a.AssetClass, b.AssetClass),
			cmp.Compare(a.InstitutionName, b.InstitutionName), cmp.Compare(a.InstitutionID, b.InstitutionID),
			b.Amount.Cmp(a.Amount), cmp.Compare(a.AccountName, b.AccountName), cmp.Compare(a.AccountID, b.AccountID))
	})
	const rootID = "assets"
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
	for _, path := range out.Paths {
		classID := rootID + "/" + path.AssetClass
		institutionID := classID + "/institution:" + path.InstitutionID
		accountID := institutionID + "/account:" + path.AccountID
		addNode(classID, rootID, "assetClass", path.AssetClass, path.Amount)
		addNode(institutionID, classID, "institution", path.InstitutionID, path.Amount)
		addNode(accountID, institutionID, "account", path.AccountID, path.Amount)
	}
	slices.SortFunc(out.Paths, func(a, b domain.OverviewAssetPath) int {
		return cmp.Or(b.Amount.Cmp(a.Amount), cmp.Compare(a.AssetClass, b.AssetClass), cmp.Compare(a.AccountID, b.AccountID))
	})
	return out
}
