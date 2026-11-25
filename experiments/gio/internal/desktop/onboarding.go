package desktop

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type onboardingService interface {
	Bootstrap(context.Context) (application.Bootstrap, error)
	CompleteOnboarding(context.Context, application.OnboardingInput) error
}

const onboardingDefaultCurrency domain.CurrencyCode = "CNY"

type onboardingResult struct {
	wrote     bool
	household bool
	err       error
}

type onboardingMember struct {
	name   ui.TextField
	remove widget.Clickable
}

type onboardingPage struct {
	ctx        context.Context
	service    onboardingService
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	results    chan onboardingResult

	reading, writing, readFailed, form, opened bool
	attempted, confirmed                       bool
	problem                                    i18n.MessageID

	name                           ui.TextField
	members                        []*onboardingMember
	membersList                    widget.List
	focusName                      bool
	focusMember                    *onboardingMember
	currency                       string
	currencyPicker, languagePicker ui.Select
	historical                     bool
	day                            string
	calendar                       ui.DatePicker
	toggle, add, create, retry     widget.Clickable
}

func newOnboardingPage(ctx context.Context, service onboardingService, clock func() time.Time, invalidate func()) *onboardingPage {
	return &onboardingPage{
		ctx: ctx, service: service, clock: clock, invalidate: invalidate,
		results: make(chan onboardingResult, 1), name: ui.NewTextField(true), currency: onboardingDefaultCurrency.String(), focusName: true,
		members:     []*onboardingMember{{name: ui.NewTextField(true)}},
		membersList: widget.List{List: layout.List{Axis: layout.Vertical}},
	}
}

func (p *onboardingPage) read() {
	if p.reading || p.writing || p.ctx.Err() != nil {
		return
	}
	p.reading, p.readFailed, p.form = true, false, false
	p.workers.Go(func() {
		core, err := p.service.Bootstrap(p.ctx)
		p.results <- onboardingResult{household: core.Household != nil, err: err}
		p.invalidate()
	})
}

// update advances the gate only with an authoritative read. Even a write
// error can follow the household transaction (e.g. default directory setup).
func (p *onboardingPage) update() {
	select {
	case result := <-p.results:
		if result.wrote {
			p.writing, p.attempted, p.confirmed = false, true, result.err == nil
			p.problem = onboardingWriteProblem(result.err)
			p.read()
			return
		}
		p.reading = false
		p.readFailed = result.err != nil || (p.confirmed && !result.household)
		if !p.readFailed {
			p.opened, p.form = result.household, !result.household
		}
	default:
	}
}

func onboardingWriteProblem(err error) i18n.MessageID {
	if err == nil {
		return i18n.NoMessage
	}
	var problem *domain.Error
	if errors.As(err, &problem) {
		switch problem.Field {
		case "startDate", "date", "timezone":
			return i18n.OnboardingDateInvalid
		case "name":
			if problem.MaxLength > 0 {
				return i18n.OnboardingNameTooLong
			}
		}
	}
	return i18n.OnboardingWriteFailed
}

func (p *onboardingPage) today(location *time.Location) string {
	return p.clock().In(location).Format(time.DateOnly)
}

func (p *onboardingPage) submit(location *time.Location) {
	if !p.form || p.writing || p.ctx.Err() != nil {
		return
	}
	input := application.OnboardingInput{HouseholdName: strings.TrimSpace(p.name.Text()), BaseCurrency: p.currency}
	if input.HouseholdName == "" {
		p.problem, p.focusName = i18n.OnboardingNameRequired, true
		return
	}
	for _, member := range p.members {
		if name := strings.TrimSpace(member.name.Text()); name != "" {
			input.MemberNames = append(input.MemberNames, name)
		}
	}
	if len(input.MemberNames) == 0 {
		p.problem, p.focusMember = i18n.OnboardingMemberRequired, p.members[0]
		return
	}
	if p.historical {
		input.Timezone = location.String()
		input.HistoryStartDate = p.day
		if input.HistoryStartDate == "" {
			input.HistoryStartDate = p.today(location)
		}
	}
	p.writing, p.problem = true, i18n.NoMessage
	p.workers.Go(func() {
		err := p.service.CompleteOnboarding(p.ctx, input)
		p.results <- onboardingResult{wrote: true, err: err}
		p.invalidate()
	})
}
