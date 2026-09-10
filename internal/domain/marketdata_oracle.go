package domain

import (
	"sort"
	"time"
)

const MarketDataResolverPolicy = "household-cutoff-close-v1"

type OracleClose struct {
	MarketDate         string
	Close              string
	SessionTimezone    string
	CloseClock         string
	SessionKind        SessionKind
	Revision           int
	SupersedesRevision int
}

type OracleHolding struct {
	Quantity      string
	QuoteCurrency CurrencyCode
}

type OracleCash struct {
	Amount   string
	Currency CurrencyCode
}

type OracleFX struct {
	BaseCurrency  CurrencyCode
	QuoteCurrency CurrencyCode
	Rate          string
}

type OracleScenario struct {
	ID                 string
	Clock              MarketDataClock
	BaseCurrency       CurrencyCode
	StartingPointLocal time.Time
	Holding            OracleHolding
	Cash               OracleCash
	FX                 OracleFX
	Closes             []OracleClose
	PendingMarketDates []string
}

type OracleSnapshot struct {
	LocalDate          string
	CutoffAt           time.Time
	EligibleMarketDate string
	EligibleClose      string
	EligibleRevision   int
	ValueEffectiveAt   time.Time
	NativeHoldingExact string
	BaseHoldingExact   string
	BaseCashExact      string
	AssetsExact        string
	AssetsRounded      string
	Complete           bool
	CarriedForward     bool
	Quality            string
}

type OracleCorrection struct {
	MarketDate             string
	OriginalClose          string
	CorrectedClose         string
	OriginalRounded        string
	CorrectedRounded       string
	RoundedAmountUnchanged bool
	ExactHoldingChanged    bool
	NewRevisionRequired    bool
}

type OracleSlice struct {
	ScenarioID             string
	ResolverPolicy         string
	Clock                  time.Time
	HouseholdTimezone      string
	TodayLocal             string
	ClosedLocalDates       []string
	OpeningAnchorDate      string
	OpeningAnchorMissing   bool
	LiveEligibleMarketDate string
	LiveCarriedForward     bool
	Snapshots              []OracleSnapshot
	Correction             OracleCorrection
	PendingMarketDates     []string
}

// EvaluateMarketDataOracle computes independent expected amounts from
// declared economic facts. It does not read provider JSON.
func EvaluateMarketDataOracle(scenario OracleScenario) (OracleSlice, error) {
	if stringsEmpty(scenario.ID) {
		return OracleSlice{}, validation("scenarioId", "is required")
	}
	today, err := scenario.Clock.TodayLocal()
	if err != nil {
		return OracleSlice{}, err
	}
	originDate := scenario.StartingPointLocal.In(mustLocation(scenario.Clock.HouseholdTimezone)).Format("2006-01-02")
	closedDates, err := closedLocalDates(originDate, today)
	if err != nil {
		return OracleSlice{}, err
	}
	quantity, err := ParseQuantity(scenario.Holding.Quantity)
	if err != nil {
		return OracleSlice{}, err
	}
	cash, err := ParseMoney(scenario.Cash.Amount, scenario.Cash.Currency)
	if err != nil {
		return OracleSlice{}, err
	}
	fx, err := ParseFxRate(scenario.FX.Rate)
	if err != nil {
		return OracleSlice{}, err
	}
	resolved, err := resolveCloses(scenario.Closes)
	if err != nil {
		return OracleSlice{}, err
	}
	snapshots := make([]OracleSnapshot, 0, len(closedDates))
	for _, localDate := range closedDates {
		cutoff, cutoffErr := HouseholdDayCutoff(localDate, scenario.Clock.HouseholdTimezone)
		if cutoffErr != nil {
			return OracleSlice{}, cutoffErr
		}
		snapshot, snapshotErr := expectedSnapshot(localDate, cutoff, quantity, cash, fx, scenario.BaseCurrency, resolved, scenario.PendingMarketDates)
		if snapshotErr != nil {
			return OracleSlice{}, snapshotErr
		}
		snapshots = append(snapshots, snapshot)
	}
	liveCutoff := scenario.Clock.Now
	live, err := expectedSnapshot(today, liveCutoff, quantity, cash, fx, scenario.BaseCurrency, resolved, scenario.PendingMarketDates)
	if err != nil {
		return OracleSlice{}, err
	}
	correction, err := expectedCorrection(scenario.Closes, quantity, fx, scenario.BaseCurrency)
	if err != nil {
		return OracleSlice{}, err
	}
	anchor, anchorMissing := "", true
	if len(snapshots) > 0 && snapshots[0].EligibleMarketDate != "" {
		anchor = snapshots[0].EligibleMarketDate
		anchorMissing = false
	}
	pending := append([]string{}, scenario.PendingMarketDates...)
	sort.Strings(pending)
	return OracleSlice{
		ScenarioID:             scenario.ID,
		ResolverPolicy:         MarketDataResolverPolicy,
		Clock:                  scenario.Clock.Now.UTC(),
		HouseholdTimezone:      scenario.Clock.HouseholdTimezone,
		TodayLocal:             today,
		ClosedLocalDates:       closedDates,
		OpeningAnchorDate:      anchor,
		OpeningAnchorMissing:   anchorMissing,
		LiveEligibleMarketDate: live.EligibleMarketDate,
		LiveCarriedForward:     live.CarriedForward,
		Snapshots:              snapshots,
		Correction:             correction,
		PendingMarketDates:     pending,
	}, nil
}

type resolvedClose struct {
	MarketDate       string
	Close            UnitPrice
	ValueEffectiveAt time.Time
	Revision         int
}

func resolveCloses(closes []OracleClose) ([]resolvedClose, error) {
	resolved := make([]resolvedClose, 0, len(closes))
	for _, item := range closes {
		evidence := SessionEvidence{Kind: item.SessionKind, Timezone: item.SessionTimezone, CloseClock: item.CloseClock, Policy: USEquityRegularClosePolicy}
		session, err := ResolveEquitySessionClose(item.MarketDate, "US", evidence)
		if err != nil {
			return nil, err
		}
		if session.Status != "mapped" {
			return nil, validation("session", "declared close session could not be resolved")
		}
		price, err := ParseUnitPrice(item.Close)
		if err != nil {
			return nil, err
		}
		revision := item.Revision
		if revision == 0 {
			revision = 1
		}
		resolved = append(resolved, resolvedClose{
			MarketDate:       item.MarketDate,
			Close:            price,
			ValueEffectiveAt: session.CloseInstant,
			Revision:         revision,
		})
	}
	sort.Slice(resolved, func(i, j int) bool {
		if resolved[i].ValueEffectiveAt.Equal(resolved[j].ValueEffectiveAt) {
			return resolved[i].Revision < resolved[j].Revision
		}
		return resolved[i].ValueEffectiveAt.Before(resolved[j].ValueEffectiveAt)
	})
	return resolved, nil
}

func expectedSnapshot(localDate string, cutoff time.Time, quantity Quantity, cash Money, fx FxRate, base CurrencyCode, closes []resolvedClose, pending []string) (OracleSnapshot, error) {
	selected, ok := latestEligibleClose(closes, cutoff)
	if !ok {
		return OracleSliceSnapshotMissing(localDate, cutoff)
	}
	native, err := MultiplyQuantityAndUnitPrice(quantity, selected.Close)
	if err != nil {
		return OracleSnapshot{}, err
	}
	baseHolding, err := MultiplyByFxRate(native, fx)
	if err != nil {
		return OracleSnapshot{}, err
	}
	cashExact, err := MultiplyByFxRate(cash.Amount(), fx)
	if err != nil {
		return OracleSnapshot{}, err
	}
	assetsExact := baseHolding.Add(cashExact)
	assetsRounded, err := NewMoney(assetsExact, base)
	if err != nil {
		return OracleSnapshot{}, err
	}
	nativeExact, err := ParseNativeAmount(canonicalDecimal(native))
	if err != nil {
		return OracleSnapshot{}, err
	}
	holdingExact, err := ParseNativeAmount(canonicalDecimal(baseHolding))
	if err != nil {
		return OracleSnapshot{}, err
	}
	cashExactText, err := ParseNativeAmount(canonicalDecimal(cashExact))
	if err != nil {
		return OracleSnapshot{}, err
	}
	assetsExactText, err := ParseNativeAmount(canonicalDecimal(assetsExact))
	if err != nil {
		return OracleSnapshot{}, err
	}
	carried := selected.MarketDate != localDate
	quality := "verified_close"
	if carried {
		quality = "carried_forward"
	}
	if marketDatePending(selected.MarketDate, pending) {
		quality = "pending"
	}
	return OracleSnapshot{
		LocalDate:          localDate,
		CutoffAt:           cutoff,
		EligibleMarketDate: selected.MarketDate,
		EligibleClose:      selected.Close.Canonical(),
		EligibleRevision:   selected.Revision,
		ValueEffectiveAt:   selected.ValueEffectiveAt,
		NativeHoldingExact: nativeExact,
		BaseHoldingExact:   holdingExact,
		BaseCashExact:      cashExactText,
		AssetsExact:        assetsExactText,
		AssetsRounded:      assetsRounded.CanonicalAmount(),
		Complete:           true,
		CarriedForward:     carried,
		Quality:            quality,
	}, nil
}

func OracleSliceSnapshotMissing(localDate string, cutoff time.Time) (OracleSnapshot, error) {
	return OracleSnapshot{
		LocalDate: localDate,
		CutoffAt:  cutoff,
		Complete:  false,
		Quality:   "missing",
	}, nil
}

func latestEligibleClose(closes []resolvedClose, cutoff time.Time) (resolvedClose, bool) {
	var selected resolvedClose
	found := false
	for _, item := range closes {
		if !ObservationEligible(item.ValueEffectiveAt, cutoff) {
			continue
		}
		if !found || item.ValueEffectiveAt.After(selected.ValueEffectiveAt) || (item.ValueEffectiveAt.Equal(selected.ValueEffectiveAt) && item.Revision > selected.Revision) {
			selected = item
			found = true
		}
	}
	return selected, found
}

func FindOpeningAnchor(requiredStart string, closes []OracleClose) (string, bool) {
	start, err := time.Parse("2006-01-02", requiredStart)
	if err != nil {
		return "", true
	}
	for _, days := range OpeningAnchorLookbackWindows() {
		bound := start.AddDate(0, 0, -days).Format("2006-01-02")
		selected := ""
		found := false
		for _, item := range closes {
			if item.MarketDate >= requiredStart || item.MarketDate < bound {
				continue
			}
			if !found || item.MarketDate > selected {
				selected = item.MarketDate
				found = true
			}
		}
		if found {
			return selected, false
		}
	}
	return "", true
}

func expectedCorrection(closes []OracleClose, quantity Quantity, fx FxRate, base CurrencyCode) (OracleCorrection, error) {
	var original, corrected OracleClose
	foundOriginal, foundCorrected := false, false
	for _, item := range closes {
		if item.SupersedesRevision > 0 {
			corrected = item
			foundCorrected = true
		}
	}
	if !foundCorrected {
		return OracleCorrection{}, nil
	}
	for _, item := range closes {
		if item.MarketDate == corrected.MarketDate && item.Revision == corrected.SupersedesRevision {
			original = item
			foundOriginal = true
		}
	}
	if !foundOriginal {
		return OracleCorrection{}, nil
	}
	originalPrice, err := ParseUnitPrice(original.Close)
	if err != nil {
		return OracleCorrection{}, err
	}
	correctedPrice, err := ParseUnitPrice(corrected.Close)
	if err != nil {
		return OracleCorrection{}, err
	}
	originalNative, err := MultiplyQuantityAndUnitPrice(quantity, originalPrice)
	if err != nil {
		return OracleCorrection{}, err
	}
	correctedNative, err := MultiplyQuantityAndUnitPrice(quantity, correctedPrice)
	if err != nil {
		return OracleCorrection{}, err
	}
	originalBase, err := MultiplyByFxRate(originalNative, fx)
	if err != nil {
		return OracleCorrection{}, err
	}
	correctedBase, err := MultiplyByFxRate(correctedNative, fx)
	if err != nil {
		return OracleCorrection{}, err
	}
	originalRounded, err := NewMoney(originalBase, base)
	if err != nil {
		return OracleCorrection{}, err
	}
	correctedRounded, err := NewMoney(correctedBase, base)
	if err != nil {
		return OracleCorrection{}, err
	}
	return OracleCorrection{
		MarketDate:             corrected.MarketDate,
		OriginalClose:          originalPrice.Canonical(),
		CorrectedClose:         correctedPrice.Canonical(),
		OriginalRounded:        originalRounded.CanonicalAmount(),
		CorrectedRounded:       correctedRounded.CanonicalAmount(),
		RoundedAmountUnchanged: originalRounded.CanonicalAmount() == correctedRounded.CanonicalAmount(),
		ExactHoldingChanged:    canonicalDecimal(originalBase) != canonicalDecimal(correctedBase),
		NewRevisionRequired:    true,
	}, nil
}

func closedLocalDates(originDate, todayLocal string) ([]string, error) {
	if originDate >= todayLocal {
		return nil, validation("startingPoint", "starting point must precede today")
	}
	parsedToday, err := time.Parse("2006-01-02", todayLocal)
	if err != nil {
		return nil, validation("today", "must use YYYY-MM-DD")
	}
	end := parsedToday.AddDate(0, 0, -1).Format("2006-01-02")
	return InclusiveMarketDates(originDate, end)
}

func marketDatePending(marketDate string, pending []string) bool {
	for _, item := range pending {
		if item == marketDate {
			return true
		}
	}
	return false
}

func stringsEmpty(value string) bool {
	return len(value) == 0
}

func mustLocation(timezone string) *time.Location {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return location
}
