package application

import (
	"sort"

	"github.com/shopspring/decimal"
)

// Categories are projected from leaves, never account totals. Assets and debt
// balances have separate denominators. An absent member is not a zero balance.
type HistoricalCategoryCell struct {
	Status         string   `json:"status"`
	Amount         *string  `json:"amount"`
	KnownAmount    *string  `json:"knownAmount"`
	Share          *string  `json:"share"`
	ShareBasis     string   `json:"shareBasis"`
	IncludedCount  int      `json:"includedCount"`
	MissingRowKeys []string `json:"missingRowKeys"`
}

type HistoricalCategoryComparison struct {
	Role          string                 `json:"role"`
	Dimension     string                 `json:"dimension"`
	Key           string                 `json:"key"`
	Left          HistoricalCategoryCell `json:"left"`
	Right         HistoricalCategoryCell `json:"right"`
	AmountChange  *string                `json:"amountChange"`
	ShareChangePp *string                `json:"shareChangePp"`
	AmountReasons []string               `json:"amountReasons"`
	ShareReasons  []string               `json:"shareReasons"`
}

func historicalCategoryComparisons(result HistoricalOverviewResult) []HistoricalCategoryComparison {
	comparisons := []HistoricalCategoryComparison{}
	if result.Right == nil {
		return comparisons
	}
	for _, role := range []string{"asset", "liability"} {
		for _, dimension := range []string{"byClass", "byCurrency"} {
			keys := map[string]bool{}
			for _, row := range result.Rows {
				if row.Kind == "account" || row.Role != role {
					continue
				}
				for _, cell := range []*HistoricalOverviewCell{row.Left, row.Right} {
					if cell != nil {
						keys[historicalCategoryKey(cell, dimension)] = true
					}
				}
			}
			ordered := make([]string, 0, len(keys))
			for key := range keys {
				ordered = append(ordered, key)
			}
			sort.Strings(ordered)
			for _, key := range ordered {
				item := HistoricalCategoryComparison{Role: role, Dimension: dimension, Key: key, AmountReasons: []string{}, ShareReasons: []string{}}
				item.Left = historicalCategorySide(result, role, dimension, key, false)
				item.Right = historicalCategorySide(result, role, dimension, key, true)
				for _, cell := range []HistoricalCategoryCell{item.Left, item.Right} {
					if cell.Status != "complete" {
						item.AmountReasons = appendHistoricalReason(item.AmountReasons, cell.Status)
					}
				}
				if result.Left.Currency != result.Right.Currency {
					item.AmountReasons = appendHistoricalReason(item.AmountReasons, "currency_mismatch")
				}
				for _, row := range result.Rows {
					if row.Kind == "account" || row.Role != role {
						continue
					}
					a, b := row.Left, row.Right
					matches := func(cell *HistoricalOverviewCell) bool {
						return cell != nil && historicalCategoryKey(cell, dimension) == key
					}
					if !matches(a) && !matches(b) {
						continue
					}
					if a == nil || b == nil {
						if (matches(a) && a.Included) || (matches(b) && b.Included) {
							item.AmountReasons = appendHistoricalReason(item.AmountReasons, "existence_changed")
						}
					} else if a.Included != b.Included {
						item.AmountReasons = appendHistoricalReason(item.AmountReasons, "scope_changed")
					} else if a.Included && b.Included && matches(a) != matches(b) {
						item.AmountReasons = appendHistoricalReason(item.AmountReasons, "membership_changed")
					}
				}
				if len(item.AmountReasons) == 0 {
					item.AmountChange = historicalDifference(item.Left.Amount, item.Right.Amount)
				}
				item.ShareReasons = append(item.ShareReasons, item.AmountReasons...)
				if item.Left.ShareBasis != "total" || item.Right.ShareBasis != "total" {
					item.ShareReasons = appendHistoricalReason(item.ShareReasons, "share_denominator_unavailable")
				}
				if len(item.ShareReasons) == 0 {
					item.ShareChangePp = historicalDifference(item.Left.Share, item.Right.Share)
				}
				comparisons = append(comparisons, item)
			}
		}
	}
	return comparisons
}

func historicalCategoryKey(cell *HistoricalOverviewCell, dimension string) string {
	if dimension == "byCurrency" {
		return cell.Currency
	}
	return cell.AssetClass
}

func historicalCategorySide(result HistoricalOverviewResult, role, dimension, key string, right bool) HistoricalCategoryCell {
	cell := HistoricalCategoryCell{Status: "absent", ShareBasis: "unavailable", MissingRowKeys: []string{}}
	known, total := decimal.Zero, decimal.Zero
	roleComplete, nonnegative := true, true
	found, hasKnownAmount := false, false
	for _, row := range result.Rows {
		if row.Kind == "account" || row.Role != role {
			continue
		}
		value := row.Left
		if right {
			value = row.Right
		}
		if value == nil {
			continue
		}
		matches := historicalCategoryKey(value, dimension) == key
		found = found || matches
		if !value.Included {
			continue
		}
		if matches {
			cell.IncludedCount++
		}
		if !value.Complete || value.BaseAmount == nil {
			roleComplete = false
			if matches {
				cell.MissingRowKeys = append(cell.MissingRowKeys, row.Key)
			}
		}
		if value.BaseAmount != nil {
			amount, err := decimal.NewFromString(*value.BaseAmount)
			if err != nil {
				roleComplete = false
				if matches && value.Complete {
					cell.MissingRowKeys = append(cell.MissingRowKeys, row.Key)
				}
				continue
			}
			nonnegative = nonnegative && !amount.IsNegative()
			total = total.Add(amount)
			if matches {
				known = known.Add(amount)
				hasKnownAmount = true
			}
		}
	}
	if !found {
		return cell
	}
	if cell.IncludedCount == 0 {
		cell.Status = "excluded"
		return cell
	}
	if hasKnownAmount {
		cell.KnownAmount = historicalString(known.String())
	}
	cell.Status = "complete"
	if len(cell.MissingRowKeys) > 0 {
		cell.Status = "partial"
	} else {
		cell.Amount = historicalString(known.String())
	}
	if hasKnownAmount && nonnegative && total.IsPositive() {
		cell.Share = historicalString(known.Mul(decimal.NewFromInt(100)).Div(total).Round(2).String())
		cell.ShareBasis = "known_subtotal"
		if roleComplete {
			cell.ShareBasis = "total"
		}
	}
	sort.Strings(cell.MissingRowKeys)
	return cell
}

func appendHistoricalReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
