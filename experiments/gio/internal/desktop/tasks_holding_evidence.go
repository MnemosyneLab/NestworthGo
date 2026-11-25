package desktop

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type holdingEvidenceDisclosure struct {
	button widget.Clickable
	open   bool
}

type holdingLinkedEvidence struct {
	Before    *domain.ReconciliationItem `json:"reviewedBefore"`
	Committed struct {
		Kind        string    `json:"kind"`
		EffectiveAt time.Time `json:"effectiveAt"`
		RecordedAt  time.Time `json:"recordedAt"`
		Note        *string   `json:"note"`
		Effects     []struct {
			Direction domain.EffectDirection `json:"direction"`
			Amount    string                 `json:"amount"`
			Currency  string                 `json:"currency"`
			Quantity  string                 `json:"quantity"`
			UnitCost  string                 `json:"unitCost"`
			Target    domain.EffectTarget    `json:"target"`
		} `json:"effects"`
		Resulting []struct {
			AccountID, Name, Amount, Currency, Quantity string
		} `json:"resulting"`
		Contract  *holdingJournalProduct  `json:"contract"`
		Contracts []holdingJournalProduct `json:"contracts"`
		Request   json.RawMessage         `json:"request"`
		Result    json.RawMessage         `json:"result"`
	} `json:"committed"`
}

func (e holdingLinkedEvidence) productValuationReceipt() *domain.ProductValuationReceipt {
	if e.Committed.Kind != string(domain.ProductOpValueObservation) {
		return nil
	}
	var result struct {
		Receipt *domain.ProductValuationReceipt `json:"receipt"`
	}
	if err := jsoncodec.Unmarshal(e.Committed.Result, &result); err != nil {
		return nil
	}
	return result.Receipt
}

type holdingJournalProduct struct {
	Name, Principal, Currency                      string
	MaturityOn, AnnualRate, MaturityInterest, Note *string
}

// The reconciliation ledger stores debt magnitudes. Apply the account's
// display sign to frozen balances, preserving their recorded digits.
func holdingJournalBalance(view ui.View, amount, currency string, role domain.BalanceSheetRole) string {
	if role == "" {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	if role == domain.RoleLiability && !strings.HasPrefix(amount, "-") && strings.Trim(amount, "0.") != "" {
		amount = "-" + amount
	}
	return holdingSnapshotValue(view, amount, currency)
}

func (d *holdingEvidenceDisclosure) toggle(view ui.View, show, hide string) layout.Widget {
	title := show
	if d.open {
		title = hide
	}
	return taskLink(view, &d.button, title, func() { d.open = !d.open })
}

func holdingEvidenceTime(view ui.View, at time.Time, zone *time.Location) string {
	if zone == nil {
		zone = time.UTC
	}
	prefs := settings.Settings{DateFormat: view.DateFormat, TimeFormat: view.TimeFormat}
	return format.LocalDateTimeSeconds(at.In(zone), prefs, settings.Language(view.Language)) + " (" + zone.String() + ")"
}

func holdingSnapshotValue(view ui.View, value, currency string) string {
	if value == "" {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	value = portfolioDecimal(view, value)
	if currency != "" {
		value += " " + currency
	}
	return value
}

func holdingLinkedEvidenceRows(view ui.View, evidence holdingLinkedEvidence, zone *time.Location, roles map[string]domain.BalanceSheetRole) []layout.Widget {
	l := view.Language
	fact := func(label i18n.MessageID, value string) layout.Widget {
		return view.Message(fmt.Sprintf(l.Text(label), value), view.Colors.Ink)
	}
	var rows []layout.Widget
	before := evidence.Before
	if before != nil {
		currency := before.Currency
		if before.Kind == "holding" {
			currency = ""
		}
		value := holdingSnapshotValue(view, before.Value, currency)
		if before.Kind == "scope" {
			value = l.Text(i18n.StatementScope)
		} else if before.Kind == "balance" {
			value = holdingJournalBalance(view, before.Value, before.Currency, roles[before.AccountID])
		}
		rows = append(rows, fact(i18n.HoldingReconReviewedBefore, value))
		if before.Cost != "" {
			rows = append(rows, fact(i18n.HoldingReconBasis, holdingSnapshotValue(view, before.Cost, before.Currency)))
		}
		if before.Source.AsOfDate != "" {
			rows = append(rows, fact(i18n.HoldingReconEvidenceDate, holdingEvidenceSourceDate(view, before.Source)))
		}
		if before.Source.Reference != "" {
			rows = append(rows, view.Message(before.Source.Reference, view.Colors.SecondaryInk))
		}
	}
	after := evidence.Committed
	if !after.EffectiveAt.IsZero() {
		if zone == nil {
			zone = time.UTC
		}
		rows = append(rows, view.Message(tradeTimestamp(view, after.EffectiveAt, zone), view.Colors.Ink))
	}
	if !after.RecordedAt.IsZero() {
		rows = append(rows, fact(i18n.HoldingReconRecordedAt, holdingEvidenceTime(view, after.RecordedAt, zone)))
	}
	for _, e := range after.Effects {
		switch {
		case e.Amount != "":
			label := i18n.HoldingReconAmountAdded
			if e.Direction == domain.EffectRemoved {
				label = i18n.HoldingReconAmountRemoved
			}
			rows = append(rows, fact(label, holdingSnapshotValue(view, e.Amount, e.Currency)))
		case e.Quantity != "":
			label := i18n.QuantityAdjustAdded
			if e.Direction == domain.EffectRemoved {
				label = i18n.QuantityAdjustRemoved
			}
			rows = append(rows, fact(label, holdingSnapshotValue(view, e.Quantity, "")))
		}
		if e.UnitCost != "" {
			currency := e.Currency
			if currency == "" && before != nil {
				currency = before.Currency
			}
			label := i18n.HoldingReconAcquisitionCostValue
			if e.Target == domain.EffectTargetHoldingCost {
				label = i18n.HoldingReconNewBasis
			}
			rows = append(rows, fact(label, holdingSnapshotValue(view, e.UnitCost, currency)))
		}
	}
	for _, result := range after.Resulting {
		text := holdingSnapshotValue(view, result.Quantity, "")
		if result.Amount != "" {
			account := result.AccountID
			if account == "" && before != nil {
				account = before.AccountID
			}
			text = holdingJournalBalance(view, result.Amount, result.Currency, roles[account])
		}
		if result.Name != "" {
			text = result.Name + " · " + text
		}
		rows = append(rows, fact(i18n.HoldingReconLinkedResult, text))
	}
	contracts := after.Contracts
	if after.Contract != nil {
		contracts = append(contracts, *after.Contract)
	}
	for _, contract := range contracts {
		if contract.Name != "" {
			rows = append(rows, view.Message(contract.Name, view.Colors.Ink))
		}
		rows = append(rows, fact(i18n.ProductPrincipal, holdingSnapshotValue(view, contract.Principal, contract.Currency)), productMaturity(view, contract.MaturityOn))
		if contract.AnnualRate != nil {
			if rate, err := domain.ParseAnnualRateRatio(*contract.AnnualRate); err == nil {
				rows = append(rows, fact(i18n.HoldingReconAnnualRate, portfolioDecimal(view, rate.Decimal().Shift(2).String())))
			}
		}
		if contract.Note != nil && *contract.Note != "" {
			rows = append(rows, view.Message(*contract.Note, view.Colors.SecondaryInk))
		}
	}
	if observed := evidence.productValuationReceipt(); observed != nil {
		rows = append(rows, fact(i18n.HoldingReconObservedValue,
			holdingSnapshotValue(view, observed.Amount.CanonicalAmount(), observed.Amount.Currency().String())))
	}
	if after.Kind == string(domain.ProductOpReceiveInterest) {
		rows = append(rows, holdingProductInterestEvidenceRows(view, evidence)...)
	}
	if after.Kind == string(domain.ProductOpSettle) && before != nil {
		rows = append(rows, holdingSettlementFacts(view, after.Request, before.Currency)...)
	}
	if after.Kind == string(domain.ProductOpRenew) && before != nil {
		rows = append(rows, holdingRenewalFacts(view, after.Request, before.Currency)...)
	}
	if after.Note != nil && *after.Note != "" {
		rows = append(rows, view.Message(*after.Note, view.Colors.Ink))
	}
	return rows
}

func holdingEvidenceSourceDate(view ui.View, source domain.ReconciliationSource) string {
	date := historyDay(view, source.AsOfDate)
	if source.AsOfTime != "" {
		date += " " + source.AsOfTime + " (" + source.Timezone + ")"
	}
	return date
}

func holdingAttemptRows(view ui.View, checkpoint domain.ReconciliationItem, zone *time.Location) []layout.Widget {
	l := view.Language
	var rows []layout.Widget
	for _, attempt := range checkpoint.Attempts {
		// Use the stored evidence fields, not the live item that may have
		// changed since this attempt.
		item := domain.ReconciliationItem{Kind: attempt.Kind, Currency: attempt.Currency, Value: attempt.Value, Cost: attempt.Cost,
			HoldingID: checkpoint.HoldingID, ProductID: checkpoint.ProductID,
			Outcome: attempt.Outcome, Note: attempt.Note, Source: attempt.Source, ReferenceValue: attempt.ReferenceValue}
		if checkedAt, err := time.Parse(time.RFC3339Nano, attempt.CheckedAt); err == nil {
			rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.StatementCheckedAt), holdingEvidenceTime(view, checkedAt, zone)), view.Colors.SecondaryInk))
		}
		if item.Kind != "scope" {
			rows = append(rows, holdingCheckValue(view, item))
			if item.Cost != "" {
				rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconBasis), holdingSnapshotValue(view, item.Cost, item.Currency)), view.Colors.Ink))
			}
		}
		rows = append(rows, holdingCheckEvidence(view, item)...)
	}
	return rows
}

func holdingCheckpointFacts(view ui.View, item domain.ReconciliationItem) []layout.Widget {
	var rows []layout.Widget
	if item.Kind == "scope" {
		return nil
	}
	if item.Kind == "removed" {
		rows = append(rows, view.Message(view.Language.Text(i18n.HoldingReconRemoved), view.Colors.SecondaryInk))
	}
	if item.Kind == "product" || item.Kind == "removed" && item.ProductID != "" {
		return append(rows, holdingProductEvidence(view, item)...)
	}
	rows = append(rows, holdingCheckValue(view, item))
	if item.Kind == "removed" && item.Cost != "" {
		cost := holdingSnapshotValue(view, item.Cost, item.Currency)
		rows = append(rows, view.Message(fmt.Sprintf(view.Language.Text(i18n.HoldingReconBasis), cost), view.Colors.Ink))
	}
	return rows
}

func holdingProductEvidence(view ui.View, item domain.ReconciliationItem) []layout.Widget {
	l := view.Language
	fact := func(label i18n.MessageID, value string) layout.Widget {
		return view.Message(fmt.Sprintf(l.Text(label), value), view.Colors.Ink)
	}
	rows := []layout.Widget{fact(i18n.ProductPrincipal, holdingSnapshotValue(view, item.Value, item.Currency))}
	var contract domain.ProductContract
	if err := jsoncodec.Unmarshal(item.Detail, &contract); err != nil {
		return append(rows, view.Message(l.Text(i18n.ProductUnavailable), view.Colors.SecondaryInk))
	}
	state := i18n.OverviewNoValue
	switch contract.State {
	case domain.ProductStateOpen:
		state = i18n.HoldingReconProductOpen
	case domain.ProductStateSettled:
		state = i18n.ProductSettled
	case domain.ProductStateCancelled:
		state = i18n.ProductCancelled
	}
	rows = append(rows, view.Message(l.Text(state), view.Colors.SecondaryInk),
		fact(i18n.ProductStart, historyDay(view, contract.StartOn)), productMaturity(view, contract.MaturityOn))
	mode := i18n.OverviewNoValue
	switch contract.InterestMode {
	case domain.InterestNone:
		mode = i18n.HoldingReconInterestNone
	case domain.InterestManualMaturityAmount:
		mode = i18n.HoldingReconInterestManual
	case domain.InterestSimpleAct365:
		mode = i18n.HoldingReconInterest365
	case domain.InterestSimpleAct360:
		mode = i18n.HoldingReconInterest360
	}
	rows = append(rows, view.Message(l.Text(mode), view.Colors.Ink))
	if contract.AnnualRate != nil {
		percent := portfolioDecimal(view, contract.AnnualRate.Decimal().Shift(2).String())
		rows = append(rows, fact(i18n.HoldingReconAnnualRate, percent))
	}
	if contract.MaturityInterest != nil {
		amount := holdingSnapshotValue(view, contract.MaturityInterest.CanonicalAmount(), contract.Currency.String())
		rows = append(rows, fact(i18n.HoldingReconMaturityInterest, amount))
	}
	if contract.InterestPaidThroughOn != nil {
		rows = append(rows, fact(i18n.HoldingReconPaidThrough, historyDay(view, *contract.InterestPaidThroughOn)))
	}
	if contract.Note != nil && *contract.Note != "" {
		rows = append(rows, view.Message(*contract.Note, view.Colors.SecondaryInk))
	}
	return rows
}
