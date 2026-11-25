package desktop

import (
	"fmt"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type holdingEvidenceDisclosure struct {
	button widget.Clickable
	open   bool
}

type holdingLinkedEvidence struct {
	Before    *domain.ReconciliationItem `json:"reviewedBefore"`
	Committed struct {
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
			Name, Amount, Currency, Quantity string
		} `json:"resulting"`
	} `json:"committed"`
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

func holdingLinkedEvidenceRows(view ui.View, evidence holdingLinkedEvidence, zone *time.Location) []layout.Widget {
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
		rows = append(rows, fact(i18n.HoldingReconReviewedBefore, holdingSnapshotValue(view, before.Value, currency)))
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
		value, currency := result.Quantity, ""
		if result.Amount != "" {
			value, currency = result.Amount, result.Currency
		}
		text := holdingSnapshotValue(view, value, currency)
		if result.Name != "" {
			text = result.Name + " · " + text
		}
		rows = append(rows, fact(i18n.HoldingReconLinkedResult, text))
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

func holdingAttemptRows(view ui.View, attempts []domain.ReconciliationAttempt, zone *time.Location) []layout.Widget {
	l := view.Language
	var rows []layout.Widget
	for _, attempt := range attempts {
		// Use the stored evidence fields, not the live item that may have
		// changed since this attempt.
		item := domain.ReconciliationItem{Kind: attempt.Kind, Currency: attempt.Currency, Value: attempt.Value, Cost: attempt.Cost,
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
