package application

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// HistoricalOverviewResult is a presentation read model, not a saved snapshot.
// Amounts are exact decimal strings. Nil means unknown, never zero.
type HistoricalOverviewResult struct {
	Timezone        string                   `json:"timezone"`
	OriginDate      string                   `json:"originDate"`
	LastClosedDate  string                   `json:"lastClosedDate"`
	CapturedAt      string                   `json:"capturedAt"`
	InputGeneration int                      `json:"inputGeneration"`
	ResolverPolicy  string                   `json:"resolverPolicy"`
	Left            HistoricalOverviewState  `json:"left"`
	Right           *HistoricalOverviewState `json:"right"`
	Rows            []HistoricalOverviewRow  `json:"rows"`
}

type HistoricalOverviewState struct {
	Date             string                         `json:"date"`
	CutoffAt         string                         `json:"cutoffAt"`
	Current          bool                           `json:"current"`
	Currency         string                         `json:"currency"`
	Complete         bool                           `json:"complete"`
	Assets           *string                        `json:"assets"`
	Liabilities      *string                        `json:"liabilities"`
	NetWorth         *string                        `json:"netWorth"`
	KnownAssets      string                         `json:"knownAssets"`
	KnownLiabilities string                         `json:"knownLiabilities"`
	ByClass          []HistoricalOverviewAllocation `json:"byClass"`
	ByCurrency       []HistoricalOverviewAllocation `json:"byCurrency"`
}

type HistoricalOverviewAllocation struct {
	Key    string `json:"key"`
	Amount string `json:"amount"`
	Share  string `json:"share"`
}

type HistoricalOverviewEvidence struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	EffectiveAt string `json:"effectiveAt"`
	MarketDate  string `json:"marketDate"`
	Freshness   string `json:"freshness"`
}

type HistoricalOverviewCell struct {
	Status        string                      `json:"status"`
	Included      bool                        `json:"included"`
	Complete      bool                        `json:"complete"`
	Currency      string                      `json:"currency"`
	NativeAmount  *string                     `json:"nativeAmount"`
	BaseAmount    *string                     `json:"baseAmount"`
	Quantity      *string                     `json:"quantity"`
	AssetClass    string                      `json:"assetClass"`
	Manual        bool                        `json:"manual"`
	ValueSourceAt string                      `json:"valueSourceAt"`
	ValueSourceID string                      `json:"valueSourceId"`
	Price         *HistoricalOverviewEvidence `json:"price"`
	FX            *HistoricalOverviewEvidence `json:"fx"`
	Missing       []string                    `json:"missing"`
}

type HistoricalOverviewRow struct {
	Key            string                  `json:"key"`
	ParentKey      string                  `json:"parentKey"`
	Kind           string                  `json:"kind"`
	Name           string                  `json:"name"`
	Role           string                  `json:"role"`
	Left           *HistoricalOverviewCell `json:"left"`
	Right          *HistoricalOverviewCell `json:"right"`
	BaseChange     *string                 `json:"baseChange"`
	NativeChange   *string                 `json:"nativeChange"`
	QuantityChange *string                 `json:"quantityChange"`
	Changed        bool                    `json:"changed"`
}

// HistoricalOverview uses one repository read transaction for both sides. It
// deliberately bypasses snapshot ensuring, provider refresh, and write services.
// Empty date chooses the last month end within the retained history window.
func (s *Service) HistoricalOverview(ctx context.Context, date, compareTo string) (HistoricalOverviewResult, error) {
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return HistoricalOverviewResult{}, err
	}
	if origin == nil {
		return HistoricalOverviewResult{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "history has not started"}
	}
	now := s.clock()
	batch, err := s.repository.LoadHistoricalSnapshotBatch(ctx, origin.HouseholdID, now)
	if err != nil {
		return HistoricalOverviewResult{}, err
	}
	location, err := time.LoadLocation(batch.Origin.Timezone)
	if err != nil {
		return HistoricalOverviewResult{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "invalid history timezone"}
	}
	today := now.In(location)
	// Date-label arithmetic must not normalize a missing local midnight.
	civilToday := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	todayDate := civilToday.Format("2006-01-02")
	originDate := batch.Origin.StartedAt.In(location).Format("2006-01-02")
	lastClosed := civilToday.AddDate(0, 0, -1).Format("2006-01-02")
	if date == "" {
		date = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1).Format("2006-01-02")
		if date < originDate {
			date = originDate
		}
	}
	cutoffFor := func(value string) (time.Time, error) {
		parsed, parseErr := time.Parse("2006-01-02", value)
		if parseErr != nil || parsed.Format("2006-01-02") != value || value < originDate || value >= todayDate {
			return time.Time{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "localDate", Message: "select a closed day on or after the history origin"}
		}
		return historicalOverviewDayCutoff(parsed, location)
	}
	cutoff, err := cutoffFor(date)
	if err != nil {
		return HistoricalOverviewResult{}, err
	}
	result := HistoricalOverviewResult{Timezone: batch.Origin.Timezone, OriginDate: originDate, LastClosedDate: lastClosed, CapturedAt: now.UTC().Format(time.RFC3339Nano), InputGeneration: batch.InputGeneration, ResolverPolicy: domain.MarketDataResolverPolicy}
	fxProvider, ttl := s.FXProviderKey(), s.QuoteCacheTTL()
	left, leftRows, err := s.historicalOverviewSide(ctx, &batch, date, cutoff, false, fxProvider, ttl)
	if err != nil {
		return HistoricalOverviewResult{}, err
	}
	result.Left = left
	var rightRows []HistoricalOverviewRow
	if compareTo != "" {
		current := compareTo == "current"
		rightCutoff := now
		if !current {
			rightCutoff, err = cutoffFor(compareTo)
		}
		if err != nil {
			return HistoricalOverviewResult{}, err
		}
		right, rows, sideErr := s.historicalOverviewSide(ctx, &batch, compareTo, rightCutoff, current, fxProvider, ttl)
		if sideErr != nil {
			return HistoricalOverviewResult{}, sideErr
		}
		result.Right, rightRows = &right, rows
	}
	result.Rows = alignHistoricalOverview(leftRows, rightRows, result.Right != nil)
	return result, nil
}

// historicalOverviewDayCutoff resolves the last instant belonging to a civil
// date, rather than resolving a possibly missing or repeated midnight. day is
// a strictly parsed UTC date label, not an instant in the household timezone.
// This is intentionally separate from transaction-time resolution, which must
// continue to reject ambiguous or nonexistent user-entered wall times.
func historicalOverviewDayCutoff(day time.Time, location *time.Location) (time.Time, error) {
	nextDay := day.AddDate(0, 0, 1)
	// IANA UTC offsets fit inside this window, including date-line changes.
	windowEnd := nextDay.Add(48 * time.Hour)
	var lastEnd time.Time
	for cursor := day.Add(-48 * time.Hour); cursor.Before(windowEnd); {
		local := cursor.In(location)
		_, offset := local.Zone()
		_, zoneEnd := local.ZoneBounds()
		if zoneEnd.IsZero() || zoneEnd.After(windowEnd) {
			zoneEnd = windowEnd
		}
		// Some runtime/tzdata combinations do not advance at a future POSIX
		// year boundary. Never loop forever or invent an offset across it.
		if !zoneEnd.After(cursor) {
			return time.Time{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "timezone", Message: "cannot resolve the day boundary: timezone interval did not advance"}
		}
		// Intersect this constant-offset interval with the requested wall date.
		start := day.Add(-time.Duration(offset) * time.Second)
		end := nextDay.Add(-time.Duration(offset) * time.Second)
		if start.Before(cursor) {
			start = cursor
		}
		if end.After(zoneEnd) {
			end = zoneEnd
		}
		if start.Before(end) && end.After(lastEnd) {
			lastEnd = end
		}
		cursor = zoneEnd
	}
	if lastEnd.IsZero() {
		return time.Time{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "localDate", Message: "the selected civil date does not exist in the household timezone"}
	}
	return lastEnd.Add(-time.Millisecond), nil
}

func (s *Service) historicalOverviewSide(ctx context.Context, batch *domain.HistoricalSnapshotBatch, date string, cutoff time.Time, current bool, fxProvider string, ttl time.Duration) (HistoricalOverviewState, []HistoricalOverviewRow, error) {
	portfolio := batch.Portfolio
	var err error
	if !current {
		portfolio, err = (HistoricalReplay{repository: s.repository, batch: batch}).Snapshot(ctx, &batch.Origin, cutoff)
		if err != nil {
			return HistoricalOverviewState{}, nil, err
		}
	}
	valuation := NewValuationService(s.repository, func() time.Time { return cutoff })
	valuation.SetFXProviderKey(func() string { return fxProvider })
	valuation.SetQuoteCacheTTL(func() time.Duration { return ttl })
	valuation.SetHistorical(!current)
	valuation.SetHistoricalMarketDate(date)
	// Value retained archived rows for inspection, but never count them. Copy
	// slices first: both sides must continue to share immutable batch inputs.
	valuedPortfolio := portfolio
	valuedPortfolio.Accounts = append([]domain.AccountRecord{}, portfolio.Accounts...)
	valuedPortfolio.Holdings = append([]domain.Holding{}, portfolio.Holdings...)
	for i := range valuedPortfolio.Accounts {
		valuedPortfolio.Accounts[i].Account.ArchivedAt = nil
	}
	for i := range valuedPortfolio.Holdings {
		valuedPortfolio.Holdings[i].ArchivedAt = nil
	}
	valued, _, err := valuation.evaluate(valuedPortfolio)
	if err != nil {
		return HistoricalOverviewState{}, nil, err
	}
	accounts := map[domain.AccountID]domain.AccountRecord{}
	for _, a := range portfolio.Accounts {
		accounts[a.Account.ID] = a
	}
	holdings := map[domain.HoldingID]domain.Holding{}
	for _, h := range portfolio.Holdings {
		holdings[h.ID] = h
	}
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for _, i := range portfolio.Instruments {
		instruments[i.ID] = i
	}
	state := HistoricalOverviewState{Date: date, CutoffAt: cutoff.UTC().Format(time.RFC3339Nano), Current: current, Currency: portfolio.Household.BaseCurrency.String(), Complete: true}
	assets, debts := decimal.Zero, decimal.Zero
	assetComplete, debtComplete := true, true
	classes, currencies := map[string]decimal.Decimal{}, map[string]decimal.Decimal{}
	rows := []HistoricalOverviewRow{}
	for _, valuedAccount := range valued {
		account := valuedAccount.model
		record := accounts[account.Account.ID]
		a := record.Account
		parentKey := a.ID.String()
		included := domain.AccountEligibleForNetWorth(a)
		parent := HistoricalOverviewCell{Status: "active", Included: included, Complete: true, Missing: []string{}}
		if a.ArchivedAt != nil {
			parent.Status = "archived"
		}
		parentIndex := len(rows)
		rows = append(rows, HistoricalOverviewRow{Key: parentKey, Kind: "account", Name: a.Name, Role: string(a.BalanceSheetRole)})
		accountSum := decimal.Zero
		for componentIndex, component := range account.Components {
			cell := HistoricalOverviewCell{Status: "active", Included: included, Complete: component.Available, Currency: component.NativeCurrency.String(), NativeAmount: historicalString(component.NativeAmount), BaseAmount: historicalString(component.BaseAmountExact), Missing: []string{}}
			kind, name, key := "balance", a.Name, parentKey+":balance"
			componentArchived := false
			var instrument *domain.Instrument
			if component.InstrumentID != nil {
				if value, ok := instruments[*component.InstrumentID]; ok {
					instrument = &value
				}
			}
			if component.HoldingID != nil {
				kind, name, key = "holding", component.InstrumentName, parentKey+":holding:"+component.HoldingID.String()
				holding := holdings[*component.HoldingID]
				cell.Quantity = historicalString(holding.Quantity.Canonical())
				componentArchived = holding.ArchivedAt != nil
				if name == "" {
					name = component.HoldingID.String()
				}
				if holding.Quantity.IsZero() {
					cell.Status = "zero"
					if historicalHoldingWasFunded(batch, holding.ID, cutoff) {
						cell.Status = "cleared"
					}
				}
			} else if a.TrackingMode == domain.TrackingHoldings {
				kind, name, key = "cash", cell.Currency, parentKey+":cash:"+cell.Currency
			}
			if componentArchived || a.ArchivedAt != nil {
				cell.Status, cell.Included = "archived", false
			}
			class, classErr := domain.ClassifyAccountComponent(a, instrument, kind == "cash")
			if classErr != nil {
				return HistoricalOverviewState{}, nil, classErr
			}
			cell.AssetClass = class.Bucket
			// Keep the originating component's missing inputs. Account-level FX
			// summaries are deduplicated and cannot be assigned back by currency:
			// a zero holding needs no FX even beside an archived foreign position.
			for _, missing := range valuedAccount.componentMissing[componentIndex] {
				cell.Missing = append(cell.Missing, string(missing.Kind))
			}
			cell.Complete = cell.Complete && len(cell.Missing) == 0
			if cell.NativeAmount == nil && cell.Status == "active" {
				cell.Status = "unknown"
			}
			if cell.NativeAmount != nil && *cell.NativeAmount == "0" && cell.Status == "active" {
				cell.Status = "zero"
			}
			cell.Price = historicalPriceEvidence(component.PriceEvidence, portfolio.InstrumentQuotes, cutoff, !current, ttl)
			cell.FX = historicalFXEvidence(component.FXEvidence, portfolio.FXQuotes, cutoff, !current, ttl)
			cell.Manual = a.TrackingMode == domain.TrackingManualValue || (cell.Price != nil && cell.Price.Source == "manual")
			if kind == "balance" || kind == "cash" {
				cell.ValueSourceAt, cell.ValueSourceID = historicalBalanceSource(batch, a.ID, cell.Currency, kind == "cash", cutoff)
			}
			rows = append(rows, HistoricalOverviewRow{Key: key, ParentKey: parentKey, Kind: kind, Name: name, Role: string(a.BalanceSheetRole), Left: &cell})
			if !componentArchived {
				parent.Complete = parent.Complete && cell.Complete && cell.BaseAmount != nil
				parent.Missing = append(parent.Missing, cell.Missing...)
				if cell.BaseAmount != nil {
					amount, parseErr := decimal.NewFromString(*cell.BaseAmount)
					if parseErr != nil {
						return HistoricalOverviewState{}, nil, parseErr
					}
					accountSum = accountSum.Add(amount)
					if included {
						if a.IsLiability() {
							debts = debts.Add(amount)
						} else {
							assets = assets.Add(amount)
							classes[cell.AssetClass] = classes[cell.AssetClass].Add(amount)
							currencies[cell.Currency] = currencies[cell.Currency].Add(amount)
						}
					}
				}
			}
			if kind == "balance" {
				parent.NativeAmount, parent.Currency, parent.Manual = cell.NativeAmount, cell.Currency, cell.Manual
				parent.ValueSourceAt, parent.ValueSourceID = cell.ValueSourceAt, cell.ValueSourceID
			}
		}
		if parent.Complete {
			parent.BaseAmount = historicalString(accountSum.String())
		}
		if parent.Status == "active" && !parent.Complete {
			parent.Status = "unknown"
		}
		if parent.Status == "active" && accountSum.IsZero() {
			parent.Status = "zero"
		}
		rows[parentIndex].Left = &parent
		if included && !parent.Complete {
			if a.IsLiability() {
				debtComplete = false
			} else {
				assetComplete = false
			}
		}
	}
	state.KnownAssets, state.KnownLiabilities = assets.String(), debts.String()
	if assetComplete {
		state.Assets = historicalString(assets.String())
	}
	if debtComplete {
		state.Liabilities = historicalString(debts.String())
	}
	state.Complete = assetComplete && debtComplete
	if state.Complete {
		state.NetWorth = historicalString(assets.Sub(debts).String())
	}
	state.ByClass, state.ByCurrency = historicalAllocations(classes, assets), historicalAllocations(currencies, assets)
	return state, rows, nil
}

func historicalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func historicalAllocations(values map[string]decimal.Decimal, total decimal.Decimal) []HistoricalOverviewAllocation {
	result := make([]HistoricalOverviewAllocation, 0, len(values))
	for key, amount := range values {
		share := "0"
		if !total.IsZero() {
			share = amount.Mul(decimal.NewFromInt(100)).Div(total).Round(2).String()
		}
		result = append(result, HistoricalOverviewAllocation{Key: key, Amount: amount.String(), Share: share})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func historicalHoldingWasFunded(batch *domain.HistoricalSnapshotBatch, id domain.HoldingID, cutoff time.Time) bool {
	for _, c := range batch.OriginData.Components {
		if c.HoldingID != nil && *c.HoldingID == id && c.Quantity != nil && !c.Quantity.IsZero() {
			return true
		}
	}
	for _, a := range batch.Activities {
		if !a.EffectiveAt.After(cutoff) {
			for _, e := range a.Effects {
				if e.HoldingID != nil && *e.HoldingID == id && e.Quantity != nil && !e.Quantity.IsZero() && e.Direction == domain.EffectAdded {
					return true
				}
			}
		}
	}
	return false
}

// Replay synthesizes AccountValue.EffectiveAt at the cutoff. Read source dates
// from origin/economic activities instead, including zero-delta observations.
func historicalBalanceSource(batch *domain.HistoricalSnapshotBatch, id domain.AccountID, currency string, cash bool, cutoff time.Time) (string, string) {
	var at time.Time
	source := ""
	target := domain.EffectTargetAccountValue
	kind := domain.HistoryOriginAccountValue
	if cash {
		target, kind = domain.EffectTargetAccountCash, domain.HistoryOriginAccountCash
	}
	for _, c := range batch.OriginData.Components {
		if c.Kind == kind && c.AccountID != nil && *c.AccountID == id && c.Amount != nil && c.Amount.Currency().String() == currency {
			at, source = batch.Origin.StartedAt, c.ID.String()
		}
	}
	for _, b := range batch.ZeroAccountBaselines {
		if !cash && b.AccountID == id && !b.EffectiveAt.After(cutoff) && b.EffectiveAt.After(at) {
			at, source = b.EffectiveAt, "baseline"
		}
	}
	for _, a := range batch.Activities {
		if a.EffectiveAt.After(cutoff) {
			continue
		}
		matches := false
		for _, e := range a.Effects {
			if e.AccountID != nil && *e.AccountID == id && e.Target == target && e.Money != nil && e.Money.Currency().String() == currency {
				matches = true
			}
		}
		for _, e := range a.Resulting {
			if e.AccountID != nil && *e.AccountID == id && e.Target == target && e.Currency.String() == currency {
				matches = true
			}
		}
		if matches && !a.EffectiveAt.Before(at) {
			at, source = a.EffectiveAt, a.ID.String()
		}
	}
	if at.IsZero() {
		return "", ""
	}
	return at.UTC().Format(time.RFC3339Nano), source
}

func historicalPriceEvidence(e *domain.QuoteEvidenceView, quotes []domain.InstrumentQuote, cutoff time.Time, historical bool, ttl time.Duration) *HistoricalOverviewEvidence {
	if e == nil {
		return nil
	}
	r := &HistoricalOverviewEvidence{ID: e.ObservationID, Source: string(e.Source), Freshness: string(e.Freshness), EffectiveAt: e.QuotedAt.UTC().Format(time.RFC3339Nano)}
	for _, q := range quotes {
		if q.ID.String() == e.ObservationID {
			at := q.ValueEffectiveAt
			if at.IsZero() {
				at = q.QuotedAt
			}
			r.EffectiveAt, r.MarketDate = at.UTC().Format(time.RFC3339Nano), q.EffectiveDate
			// Historical close revisions may have been fetched later. Freshness
			// describes the effective observation, never its later ingestion time.
			if historical {
				evidenceCutoff := cutoff
				if at.After(cutoff) && q.EffectiveDate != "" && q.SourceKind != domain.QuoteSourceManual {
					evidenceCutoff = at
				}
				r.Freshness = string(domain.QuoteFreshness(q.SourceKind, q.Delayed, at, evidenceCutoff, ttl))
			}
			break
		}
	}
	return r
}

func historicalFXEvidence(e *domain.QuoteEvidenceView, quotes []domain.FXQuote, cutoff time.Time, historical bool, ttl time.Duration) *HistoricalOverviewEvidence {
	if e == nil {
		return nil
	}
	r := &HistoricalOverviewEvidence{ID: e.ObservationID, Source: string(e.Source), Freshness: string(e.Freshness), EffectiveAt: e.QuotedAt.UTC().Format(time.RFC3339Nano)}
	for _, q := range quotes {
		if q.ID.String() == e.ObservationID {
			at := q.ValueEffectiveAt
			if at.IsZero() {
				at = q.QuotedAt
			}
			r.EffectiveAt, r.MarketDate = at.UTC().Format(time.RFC3339Nano), q.EffectiveDate
			if historical {
				evidenceCutoff := cutoff
				if at.After(cutoff) && q.EffectiveDate != "" && q.SourceKind != domain.QuoteSourceManual {
					evidenceCutoff = at
				}
				r.Freshness = string(domain.QuoteFreshness(q.SourceKind, q.Delayed, at, evidenceCutoff, ttl))
			}
			break
		}
	}
	return r
}

func alignHistoricalOverview(left, right []HistoricalOverviewRow, comparing bool) []HistoricalOverviewRow {
	byKey := map[string]HistoricalOverviewRow{}
	for _, row := range left {
		byKey[row.Key] = row
	}
	for _, row := range right {
		merged, found := byKey[row.Key]
		if !found {
			merged = row
			merged.Left = nil
		}
		merged.Right = row.Left
		byKey[row.Key] = merged
	}
	changedParents := map[string]bool{}
	for key, row := range byKey {
		row.Changed = comparing && !historicalCellsEqual(row.Left, row.Right)
		if row.Left != nil && row.Right != nil {
			if row.Left.Complete && row.Right.Complete {
				row.BaseChange = historicalDifference(row.Left.BaseAmount, row.Right.BaseAmount)
			}
			if row.Left.Currency == row.Right.Currency {
				row.NativeChange = historicalDifference(row.Left.NativeAmount, row.Right.NativeAmount)
			}
			row.QuantityChange = historicalDifference(row.Left.Quantity, row.Right.Quantity)
		}
		if row.Changed {
			changedParents[row.ParentKey] = true
		}
		byKey[key] = row
	}
	rows := make([]HistoricalOverviewRow, 0, len(byKey))
	for _, row := range byKey {
		row.Changed = row.Changed || changedParents[row.Key]
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		ap, bp := a.ParentKey, b.ParentKey
		if ap == "" {
			ap = a.Key
		}
		if bp == "" {
			bp = b.Key
		}
		if ap != bp {
			return byKey[ap].Name < byKey[bp].Name || (byKey[ap].Name == byKey[bp].Name && ap < bp)
		}
		if a.ParentKey == "" || b.ParentKey == "" {
			return a.ParentKey == ""
		}
		return a.Name < b.Name || (a.Name == b.Name && a.Key < b.Key)
	})
	return rows
}

func historicalDifference(left, right *string) *string {
	if left == nil || right == nil {
		return nil
	}
	a, errA := decimal.NewFromString(*left)
	b, errB := decimal.NewFromString(*right)
	if errA != nil || errB != nil {
		return nil
	}
	return historicalString(b.Sub(a).String())
}

// Changes-only describes balances, quantities, scope and data availability.
// A new quote ID or a repeated manual confirmation alone is not a balance change.
func historicalCellsEqual(left, right *HistoricalOverviewCell) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	a, b := *left, *right
	a.ValueSourceAt, a.ValueSourceID, b.ValueSourceAt, b.ValueSourceID = "", "", "", ""
	a.Price, a.FX, b.Price, b.FX = nil, nil, nil, nil
	return reflect.DeepEqual(a, b)
}
