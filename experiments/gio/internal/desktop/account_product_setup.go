package desktop

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *accountsPage) startProductSetup(account domain.Account, defaultName string) {
	p.productSaved = false
	run := productSetupRuntime{ctx: p.ctx, ledger: p.productLedger, clock: p.clock, workers: &p.workers, invalidate: p.invalidate}
	p.productSetup = newProductSetup(run, []domain.Account{account}, defaultName)
}

func (p *accountsPage) productSetupCalendar() productFormCalendar {
	if p.current == nil || p.current.historyZone == nil {
		return productFormCalendar{}
	}
	zone := p.current.historyZone
	return productFormCalendar{today: p.clock().In(zone).Format(time.DateOnly), zone: zone.String()}
}

func (p *accountsPage) cancelProductSetup() {
	p.productSetup, p.restoreProductFocus = nil, true
}
