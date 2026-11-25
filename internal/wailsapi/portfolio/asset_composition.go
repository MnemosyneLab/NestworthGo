package portfolio

import "github.com/waltwang/nestworth-go/internal/domain"

type AssetCompositionDTO struct {
	Positive     string         `json:"positive"`
	Negative     string         `json:"negative"`
	ZeroCount    int            `json:"zeroCount"`
	MissingCount int            `json:"missingCount"`
	Paths        []AssetPathDTO `json:"paths"`
	Nodes        []AssetNodeDTO `json:"nodes"`
}

type AssetPathDTO struct {
	AssetClass      string `json:"assetClass"`
	InstitutionID   string `json:"institutionId"`
	InstitutionName string `json:"institutionName"`
	AccountID       string `json:"accountId"`
	AccountName     string `json:"accountName"`
	Amount          string `json:"amount"`
}

type AssetNodeDTO struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	Amount   string `json:"amount"`
}

func fromAssetComposition(composition domain.OverviewAssetComposition) AssetCompositionDTO {
	out := AssetCompositionDTO{
		Positive: composition.Positive.String(), Negative: composition.Negative.String(),
		ZeroCount: composition.ZeroCount, MissingCount: composition.MissingCount,
		Paths: make([]AssetPathDTO, 0, len(composition.Paths)),
		Nodes: make([]AssetNodeDTO, 0, len(composition.Nodes)),
	}
	for _, path := range composition.Paths {
		out.Paths = append(out.Paths, AssetPathDTO{
			AssetClass: path.AssetClass, InstitutionID: path.InstitutionID, InstitutionName: path.InstitutionName,
			AccountID: path.AccountID, AccountName: path.AccountName, Amount: path.Amount.String(),
		})
	}
	for _, node := range composition.Nodes {
		out.Nodes = append(out.Nodes, AssetNodeDTO{ID: node.ID, ParentID: node.ParentID, Kind: node.Kind, Key: node.Key, Amount: node.Amount.String()})
	}
	return out
}
