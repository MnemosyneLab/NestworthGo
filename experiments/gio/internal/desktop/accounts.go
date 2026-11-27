package desktop

import (
	"context"
	"slices"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/shopspring/decimal"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountsReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	WithRead(context.Context, func(context.Context) error) error
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	AccountValuations(context.Context, domain.AccountFilter) ([]domain.AccountValuation, error)
	ListInstitutions(context.Context, bool) ([]domain.Institution, error)
	ListMembers(context.Context, bool) ([]domain.Member, error)
	ListGroups(context.Context, bool) ([]domain.Group, error)
	AccountHoldingGroups(context.Context, domain.AccountID) ([]domain.InstrumentHoldingGroupView, error)
	HoldingsByAccounts(context.Context, []domain.AccountID) (map[domain.AccountID][]domain.Holding, error)
	ListProducts(context.Context, *domain.AccountID, bool) ([]application.ProductDetail, error)
	ReconciliationLastSuccess(context.Context, string) (*domain.ReconciliationSession, error)
	ListAccountCashValues(context.Context, domain.AccountID) ([]domain.AccountCashValue, error)
}

type accountRow struct {
	record domain.AccountRecord
	value  *domain.AccountValuation
}

type accountsResult struct {
	rows          []accountRow
	institutions  map[domain.InstitutionID]domain.Institution
	members       map[domain.MemberID]domain.Member
	groups        map[domain.GroupID]domain.Group
	holdings      map[domain.HoldingID]domain.Holding
	holdingGroups []domain.InstrumentHoldingGroupView
	holdingsErr   error
	lastCheck     *domain.ReconciliationSession
	checkErr      error
	cashValues    []domain.AccountCashValue
	cashValuesErr error
	managed       map[domain.InstrumentID]bool
	products      []application.ProductDetail
	unavailable   bool
	historyZone   *time.Location
	readAt        time.Time
	err           error
}

// Match the Wails list and valuation reads. A detail visit uses the same
// projection scoped to one exact account, including archived accounts.
func readAccounts(ctx context.Context, service accountsReader, target navigationTarget, archived bool) (result accountsResult) {
	filter := domain.AccountFilter{IncludeArchived: archived}
	if target.account != "" {
		filter.IncludeArchived = true
		filter.AccountIDs = []domain.AccountID{target.account}
	} else if target.institution != "" {
		filter.InstitutionID = &target.institution
	} else if !target.accountList.filter(&filter) {
		result.unavailable = true
		return
	}
	institutions, err := service.ListInstitutions(ctx, true)
	if err != nil {
		result.err = err
		return
	}
	result.institutions = make(map[domain.InstitutionID]domain.Institution, len(institutions))
	for _, institution := range institutions {
		result.institutions[institution.ID] = institution
	}
	if filter.InstitutionID != nil && result.institutions[*filter.InstitutionID].ID == "" {
		result.unavailable = true
		return
	}
	records, err := service.ListAccounts(ctx, filter)
	if err != nil {
		result.err = err
		return
	}
	if target.account != "" {
		records = slices.DeleteFunc(records, func(r domain.AccountRecord) bool { return r.Account.ID != target.account })
		result.unavailable = len(records) == 0
	}
	if target.accountList.unassigned() {
		records = slices.DeleteFunc(records, func(r domain.AccountRecord) bool { return !target.accountList.includes(r) })
		for _, record := range records {
			filter.AccountIDs = append(filter.AccountIDs, record.Account.ID)
		}
	}
	if len(records) == 0 {
		return
	}
	if err := ctx.Err(); err != nil {
		result.err = err
		return
	}
	values, err := service.AccountValuations(ctx, filter)
	if err != nil {
		result.err = err
		return
	}
	byID := make(map[domain.AccountID]*domain.AccountValuation, len(values))
	for i := range values {
		byID[values[i].Account.ID] = &values[i]
	}
	for _, record := range records {
		result.rows = append(result.rows, accountRow{record: record, value: byID[record.Account.ID]})
	}
	if target.account == "" {
		return
	}
	members, err := service.ListMembers(ctx, true)
	if err != nil {
		result.err = err
		return
	}
	result.members = make(map[domain.MemberID]domain.Member, len(members))
	for _, member := range members {
		result.members[member.ID] = member
	}
	if records[0].Account.GroupID != nil || accountMaintenanceSupported(records[0].Account) {
		groups, err := service.ListGroups(ctx, true)
		if err != nil {
			result.err = err
			return
		}
		result.groups = make(map[domain.GroupID]domain.Group, len(groups))
		for _, group := range groups {
			result.groups[group.ID] = group
		}
	}
	result.lastCheck, result.checkErr = service.ReconciliationLastSuccess(ctx, target.account.String())
	if records[0].Account.TrackingMode == domain.TrackingHoldings {
		result.cashValues, result.cashValuesErr = service.ListAccountCashValues(ctx, target.account)
		origin, err := service.HistoryOrigin(ctx)
		if err != nil {
			result.err = err
			return
		}
		if origin != nil {
			result.historyZone, _ = time.LoadLocation(origin.Timezone)
		}
		holdings, err := service.HoldingsByAccounts(ctx, filter.AccountIDs)
		if err != nil {
			result.err = err
			return
		}
		result.holdings = make(map[domain.HoldingID]domain.Holding, len(holdings[target.account]))
		for _, holding := range holdings[target.account] {
			result.holdings[holding.ID] = holding
		}
		products, err := service.ListProducts(ctx, &target.account, true)
		if err != nil {
			result.err = err
			return
		}
		result.managed = make(map[domain.InstrumentID]bool, len(products))
		result.products = products
		for _, product := range products {
			result.managed[product.Contract.InstrumentID] = true
		}
		for _, holding := range holdings[target.account] {
			if records[0].Account.AccountType != domain.TypeCashOnHand && holding.ArchivedAt == nil && !result.managed[holding.InstrumentID] {
				result.holdingGroups, result.holdingsErr = service.AccountHoldingGroups(ctx, target.account)
				break
			}
		}
	}
	return
}

type accountListRow struct {
	heading  string
	icon     ui.Glyph
	subtotal *domain.MoneyView
	account  *accountRow
}

func (r *accountsResult) listRows(lang i18n.Language) []accountListRow {
	type institutionGroup struct {
		id         domain.InstitutionID
		label      string
		icon       ui.Glyph
		rank       decimal.Decimal
		rows       []*accountRow
		currency   domain.CurrencyCode
		incomplete bool
	}
	groups := make(map[domain.InstitutionID]*institutionGroup)
	var ordered []*institutionGroup
	for i := range r.rows {
		row := &r.rows[i]
		var id domain.InstitutionID
		if row.record.Account.InstitutionID != nil {
			id = *row.record.Account.InstitutionID
		}
		group := groups[id]
		if group == nil {
			institution := r.institutions[id]
			label := lang.Text(i18n.AccountNoInstitution)
			if id != "" {
				label = accountReference(lang, row.record.InstitutionName, institution.ArchivedAt != nil)
			}
			group = &institutionGroup{id: id, label: label}
			if id != "" {
				key := ""
				if institution.IconKey != nil {
					key = *institution.IconKey
				}
				group.icon = directoryGlyph(key, institutionFallbackIcon(institution.InstitutionType, domain.DefaultInstitutionIcon))
			}
			groups[id] = group
			ordered = append(ordered, group)
		}
		group.rows = append(group.rows, row)
		amount, valid := accountSignedBase(*row)
		group.rank = group.rank.Add(amount)
		if !valid || !row.value.Complete {
			group.incomplete = true
		} else if group.currency == "" {
			group.currency = row.value.BaseValue.Currency
		} else if group.currency != row.value.BaseValue.Currency {
			group.incomplete = true
		}
	}
	collation := collate.New(language.Make(string(lang)))
	slices.SortStableFunc(ordered, func(a, b *institutionGroup) int {
		if a.id == b.id {
			return 0
		}
		if a.id == "" {
			return 1
		}
		if b.id == "" {
			return -1
		}
		if order := b.rank.Cmp(a.rank); order != 0 {
			return order
		}
		return collation.CompareString(a.label, b.label)
	})
	var rows []accountListRow
	for _, group := range ordered {
		heading := accountListRow{heading: group.label, icon: group.icon}
		if len(group.rows) > 1 && !group.incomplete && group.currency != "" {
			heading.subtotal = &domain.MoneyView{Amount: group.rank.String(), Currency: group.currency}
		}
		rows = append(rows, heading)
		slices.SortStableFunc(group.rows, func(a, b *accountRow) int {
			if order := accountRank(*b).Cmp(accountRank(*a)); order != 0 {
				return order
			}
			return collation.CompareString(a.record.Account.Name, b.record.Account.Name)
		})
		for _, row := range group.rows {
			rows = append(rows, accountListRow{account: row})
		}
	}
	return rows
}

func accountRank(row accountRow) decimal.Decimal {
	amount, _ := accountSignedBase(row)
	return amount
}

// The same signed household-base value orders account rows and contributes to
// a subtotal only if the whole institution has a complete valuation.
func accountSignedBase(row accountRow) (decimal.Decimal, bool) {
	if row.value == nil || row.value.BaseValue == nil {
		return decimal.Zero, false
	}
	amount, err := decimal.NewFromString(row.value.BaseValue.Amount)
	if err != nil {
		return decimal.Zero, false
	}
	if row.record.Account.IsLiability() {
		amount = amount.Abs().Neg()
	}
	return amount, true
}

type accountsPage struct {
	filterLabel string
	ctx         context.Context
	service     accountsReader
	clock       func() time.Time
	invalidate  func()
	workers     sync.WaitGroup
	request     <-chan accountsResult
	cancel      context.CancelFunc

	target                             navigationTarget
	archived                           bool
	invalid                            bool
	current                            *accountsResult
	err                                error
	rows                               []accountListRow
	locale                             i18n.Language
	position                           layout.Position
	list                               widget.List
	buttons                            map[domain.AccountID]*widget.Clickable
	instruments                        map[domain.InstrumentID]*widget.Clickable
	reload, archiveFilter, clear, back widget.Clickable
	open                               func(navigationTarget, string, event.Tag)
	openInstrument                     func(domain.InstrumentID, domain.AccountID, string, event.Tag)
	openHoldingHistory                 func(domain.InstrumentID, domain.AccountID, string, event.Tag)
	openHoldingAnalysis                func(routeID, domain.InstrumentID, domain.AccountID, string, event.Tag)
	positions                          accountPositions
	detail                             accountDetailControls
	openProduct                        func(domain.ProductContractID, string, event.Tag)
	productButtons                     map[domain.ProductContractID]*widget.Clickable
	productLedger                      fundsProductLedger
	productSetup                       *fundsProductSetup
	addProduct                         widget.Clickable
	restoreProductFocus, productSaved  bool
	backToList                         func()
	creation                           *balanceAccountCreation
	createBank, createAccount          widget.Clickable
	createShell                        widget.Clickable
	focusCreate                        *widget.Clickable
	createdID                          domain.AccountID
	createdMessage                     i18n.MessageID
	maintenance                        *accountMaintenance
	cashBalance                        *accountCashBalance
	investment                         *accountInvestment
	balanceTarget                      *accountBalanceTarget
	liabilityPosting                   *accountLiabilityPosting
	statement                          *accountStatement
}

func newAccountsPage(ctx context.Context, service accountsReader, clock func() time.Time, invalidate func()) *accountsPage {
	return &accountsPage{ctx: ctx, service: service, clock: clock, invalidate: invalidate,
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, buttons: make(map[domain.AccountID]*widget.Clickable),
		instruments: make(map[domain.InstrumentID]*widget.Clickable), productButtons: make(map[domain.ProductContractID]*widget.Clickable)}
}

func (p *accountsPage) enter(target navigationTarget, archived bool, position layout.Position) {
	p.leave()
	p.target, p.archived, p.position = target, archived, position
	p.list.Position = position
	p.current, p.err, p.rows = nil, nil, nil
	p.productSetup, p.restoreProductFocus, p.productSaved = nil, false, false
	p.positions.visit = accountPositionsVisit{}
	p.detail.restore(accountCompositionVisit{})
	p.detail.investmentActionsRow, p.detail.revealedShortcut = -1, nil
	p.invalid = !validAccountTarget(target)
	p.read()
}

func validAccountTarget(target navigationTarget) bool {
	if target.accountList.active() {
		return target.account == "" && target.institution == "" && target.accountList.filter(&domain.AccountFilter{})
	}
	if target.account != "" {
		_, err := domain.ParseAccountID(target.account.String())
		return err == nil && target.institution == ""
	}
	if target.institution != "" {
		_, err := domain.ParseInstitutionID(target.institution.String())
		return err == nil
	}
	return true
}

func (p *accountsPage) leave() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.request = nil
}

func (p *accountsPage) refresh() {
	p.leave()
	p.read()
}

func (p *accountsPage) read() {
	if p.request != nil || p.invalid || p.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	result := make(chan accountsResult, 1)
	target, archived := p.target, p.archived
	p.request, p.cancel, p.err = result, cancel, nil
	p.workers.Go(func() {
		readAt := p.clock()
		var value accountsResult
		value.err = p.service.WithRead(ctx, func(ctx context.Context) error {
			value = readAccounts(ctx, p.service, target, archived)
			return value.err
		})
		if ctx.Err() == nil {
			value.readAt = readAt
			result <- value
			p.invalidate()
		}
	})
}

func (p *accountsPage) update(lang i18n.Language) bool {
	changed := false
	select {
	case result := <-p.request:
		p.leave()
		p.err = result.err
		if p.err == nil {
			if p.current != nil {
				p.position = p.list.Position
			}
			p.current = &result
			p.refreshAccountSettings()
			changed = true
		}
	default:
	}
	if p.current != nil && p.target.account == "" && (changed || p.locale != lang) {
		if !changed {
			p.position = p.list.Position
		}
		p.rows = p.current.listRows(lang)
		p.list.Position = restoredPosition(p.position, len(p.rows))
	}
	p.locale = lang
	return changed
}

func (p *accountsPage) name() string {
	if p.target.accountList.active() {
		return p.target.accountList.name(p.locale, p.filterLabel)
	}
	if p.current == nil || p.current.unavailable {
		return ""
	}
	if p.target.account != "" && len(p.current.rows) > 0 {
		return p.current.rows[0].record.Account.Name
	}
	return p.current.institutions[p.target.institution].Name
}
