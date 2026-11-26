package desktop

import (
	"context"
	"slices"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type marketAgentReader interface {
	AgentMarketDataRecords(context.Context) ([]domain.AgentQuoteRecord, error)
	ScanMarketDataHealth(context.Context) (application.MarketDataHealthReport, error)
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
}

type marketClosedDateWriter interface {
	SetInstrumentClosedDate(context.Context, string, string, string) (application.AgentMarketDataResult, error)
}

type marketAgentEvidenceRow struct {
	record   domain.AgentQuoteRecord
	day      string
	inactive bool
}

type marketAgentRead struct {
	rows       []marketAgentEvidenceRow
	findings   []application.HealthIssue
	today      string
	canDeclare bool
}

type marketAgentEvidence struct {
	target      marketQuoteHistoryTarget
	from        event.Tag
	sheet       analysisSheet
	overlay     ui.Overlay
	request     <-chan historyResult[marketAgentRead]
	stop        context.CancelFunc
	result      *marketAgentRead
	err         error
	rows        []marketAgentEvidenceRow
	findings    []application.HealthIssue
	scope       bool
	all, dated  widget.Clickable
	picker      ui.DatePicker
	date        string
	attemptDate string
	operationID string
	receipt     *application.AgentMarketDataResult
	saving      <-chan historyResult[application.AgentMarketDataResult]
	saveErr     error
}

func (p *marketDataPage) openAgentEvidence(target marketQuoteHistoryTarget, from event.Tag) {
	window := marketQuoteFindingWindow(target.finding)
	p.agentEvidence = &marketAgentEvidence{
		target: target, from: from, sheet: newAnalysisSheet(), scope: window.from != "", date: window.from,
	}
	p.readAgentEvidence()
}

func (p *marketDataPage) readAgentEvidence() {
	d := p.agentEvidence
	ctx, cancel := context.WithCancel(p.ctx)
	d.stop, d.err = cancel, nil
	target, now := d.target, p.quoteHistoryClock()
	d.request = runHistoryRead(&p.workers, p.invalidate, func() (marketAgentRead, error) {
		var data marketAgentRead
		location := time.UTC
		origin, err := p.agentReader.HistoryOrigin(ctx)
		if err != nil {
			return data, err
		}
		if origin != nil {
			if loc, err := time.LoadLocation(origin.Timezone); err == nil {
				location = loc
			}
		}
		data.today = now.In(location).Format(time.DateOnly)
		records, err := p.agentReader.AgentMarketDataRecords(ctx)
		if err != nil {
			return data, err
		}
		data.rows = projectAgentEvidence(records, target, location)
		report, err := p.agentReader.ScanMarketDataHealth(ctx)
		if err != nil {
			return data, err
		}
		focus := healthFindingFocus{instrument: target.instrument.String(), currencyA: target.pair.first.String(), currencyB: target.pair.second.String()}
		data.findings = focusedHealthFindings(report.Issues, focus)
		if target.instrument == "" || p.closedDateWriter == nil {
			return data, nil
		}
		instruments, err := p.reader.ListInstruments(ctx, true)
		if err != nil {
			return data, err
		}
		for _, instrument := range instruments {
			if instrument.ID == target.instrument && instrument.ArchivedAt == nil {
				owners, err := p.reader.InstrumentProductOwnership(ctx)
				if err != nil {
					return data, err
				}
				data.canDeclare = owners[instrument.ID.String()] == ""
				break
			}
		}
		return data, nil
	})
}

func (d *marketAgentEvidence) filter() {
	if !d.scope {
		d.rows, d.findings = d.result.rows, d.result.findings
		return
	}
	window := marketQuoteFindingWindow(d.target.finding)
	d.rows = nil
	for _, row := range d.result.rows {
		if row.day >= window.from && row.day <= window.to {
			d.rows = append(d.rows, row)
		}
	}
	d.findings = focusedHealthFindings(d.result.findings, healthFindingFocus{from: window.from, to: window.to})
}

func (d *marketAgentEvidence) name() string {
	if d.target.instrument != "" {
		return d.target.name
	}
	return d.target.pair.first.String() + "/" + d.target.pair.second.String()
}

func (p *marketDataPage) saveAgentClosedDate() {
	d := p.agentEvidence
	if d.attemptDate != d.date {
		d.operationID, d.attemptDate = domain.NewActivityID().String(), d.date
	}
	d.saveErr, d.receipt = nil, nil
	id, operation, day := d.target.instrument.String(), d.operationID, d.attemptDate
	d.saving = runHistoryRead(&p.workers, p.invalidate, func() (application.AgentMarketDataResult, error) {
		return p.closedDateWriter.SetInstrumentClosedDate(p.ctx, operation, id, day)
	})
}

// Preserve the audit and mark superseded facts. A withdrawal does not itself
// contain a quote or a closed date, so its target supplies the displayed day.
func projectAgentEvidence(records []domain.AgentQuoteRecord, target marketQuoteHistoryTarget, location *time.Location) []marketAgentEvidenceRow {
	days := make(map[string]string)
	inactive := make(map[string]bool)
	for _, record := range records {
		if record.TargetQuoteID != "" {
			inactive[record.TargetQuoteID] = true
		}
		if id, day := agentEvidenceFact(record, target, location); id != "" {
			days[id] = day
		}
	}
	var rows []marketAgentEvidenceRow
	for _, record := range slices.Backward(records) {
		if record.Operation == domain.AgentQuoteRetract {
			if day, ok := days[record.TargetQuoteID]; ok {
				rows = append(rows, marketAgentEvidenceRow{record: record, day: day})
			}
		} else if id, day := agentEvidenceFact(record, target, location); id != "" {
			rows = append(rows, marketAgentEvidenceRow{record: record, day: day, inactive: inactive[id]})
		}
	}
	return rows
}

func agentEvidenceFact(record domain.AgentQuoteRecord, target marketQuoteHistoryTarget, location *time.Location) (id, day string) {
	switch {
	case target.instrument != "":
		if q := record.Closure; q != nil && q.InstrumentID == target.instrument {
			return q.ID, q.Date
		}
		if q := record.InstrumentQuote; q != nil && q.InstrumentID == target.instrument {
			return q.ID.String(), agentEvidenceDay(q.EffectiveDate, q.QuotedAt, location)
		}
	case record.FXQuote != nil:
		q, pair := record.FXQuote, target.pair
		if q.BaseCurrency == pair.first && q.QuoteCurrency == pair.second || q.BaseCurrency == pair.second && q.QuoteCurrency == pair.first {
			return q.ID.String(), agentEvidenceDay(q.EffectiveDate, q.QuotedAt, location)
		}
	}
	return "", ""
}

func agentEvidenceDay(effective string, at time.Time, location *time.Location) string {
	if effective != "" || at.IsZero() {
		return effective
	}
	return at.In(location).Format(time.DateOnly)
}

func (d *marketAgentEvidence) selectScope(scoped bool) {
	if scoped != d.scope {
		d.scope = scoped
		d.filter()
		d.sheet.list.Position = layout.Position{}
	}
}
