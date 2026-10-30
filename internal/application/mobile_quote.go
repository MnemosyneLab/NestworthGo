package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type MobileQuoteRepository interface {
	LookupMobileQuote(context.Context, domain.HouseholdID, string) (*domain.MobileQuoteReceipt, error)
	CommitMobileQuote(context.Context, domain.MobileQuoteReceipt, *domain.InstrumentQuote, *domain.FXQuote) error
}

type MobileQuoteInput struct {
	OperationID   string `json:"operationId"`
	InstrumentID  string `json:"instrumentId,omitempty"`
	Currency      string `json:"currency,omitempty"`
	QuoteCurrency string `json:"quoteCurrency,omitempty"`
	Value         string `json:"value"`
	QuotedAt      string `json:"quotedAt"`
}

type MobileQuotePlan struct {
	Receipt    domain.MobileQuoteReceipt
	ExpiresAt  time.Time
	instrument *domain.InstrumentQuote
	fx         *domain.FXQuote
	token      string
}

func (s *Service) PreviewMobileQuote(ctx context.Context, in MobileQuoteInput, deviceID string) (*MobileQuotePlan, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if id, err := uuid.Parse(in.OperationID); err != nil || id == uuid.Nil {
		return nil, mobileInvalid("operationId")
	}
	snapshot, hh, err := s.portfolioSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if hh == nil {
		return nil, onboardingRequired()
	}
	when, err := s.parseManualQuoteTimestamp(ctx, in.QuotedAt)
	if err != nil {
		return nil, err
	}
	p := &MobileQuotePlan{Receipt: domain.MobileQuoteReceipt{OperationID: in.OperationID, HouseholdID: hh.ID, DeviceID: deviceID, QuotedAt: when, RecordedAt: s.clock()}, ExpiresAt: time.Now().Add(10 * time.Minute), token: s.writes.previewToken()}
	if len(in.QuotedAt) == 10 {
		p.Receipt.EffectiveDate = in.QuotedAt
	}
	if in.InstrumentID != "" {
		instrument, ok := instrumentFromSnapshot(snapshot, domain.InstrumentID(in.InstrumentID))
		if !ok {
			return nil, notFound("instrument")
		}
		if instrument.ArchivedAt != nil || instrument.QuoteSource != domain.QuoteSourceManual {
			return nil, mobileInvalid("instrumentId")
		}
		if err := s.rejectManagedInstrument(ctx, instrument.ID); err != nil {
			return nil, err
		}
		price, err := domain.ParseUnitPrice(in.Value)
		if err != nil {
			return nil, err
		}
		q, err := domain.NewInstrumentQuote(instrument, domain.InstrumentQuoteInput{UnitPrice: price, SourceKind: domain.QuoteSourceManual, QuotedAt: when}, s.clock())
		if err != nil {
			return nil, err
		}
		p.instrument = &q
		p.Receipt.Kind = "instrument"
		p.Receipt.FactID = q.ID.String()
		p.Receipt.InstrumentID = q.InstrumentID
		p.Receipt.Currency = q.Currency.String()
		p.Receipt.Value = q.UnitPrice.Canonical()
	} else {
		a, err := domain.ParseSupportedCurrency(in.Currency)
		if err != nil {
			return nil, err
		}
		b, err := domain.ParseSupportedCurrency(in.QuoteCurrency)
		if err != nil {
			return nil, err
		}
		manual := false
		for _, pref := range snapshot.FXPreferences {
			if (pref.CurrencyA == a && pref.CurrencyB == b || pref.CurrencyA == b && pref.CurrencyB == a) && pref.SourceKind == domain.QuoteSourceManual {
				manual = true
			}
		}
		if !manual {
			return nil, mobileInvalid("currencyPair")
		}
		rate, err := domain.ParseFxRate(in.Value)
		if err != nil {
			return nil, err
		}
		q, err := domain.NewFXQuote(domain.FXQuoteInput{HouseholdID: hh.ID, BaseCurrency: a, QuoteCurrency: b, Rate: rate, SourceKind: domain.QuoteSourceManual, QuotedAt: when}, s.clock())
		if err != nil {
			return nil, err
		}
		p.fx = &q
		p.Receipt.Kind = "fx"
		p.Receipt.FactID = q.ID.String()
		p.Receipt.Currency = a.String()
		p.Receipt.QuoteCurrency = b.String()
		p.Receipt.Value = rate.Canonical()
		p.Receipt.DerivedPending = true
	}
	// Exclude generated fact/audit identities from the canonical business hash.
	canonical := p.Receipt
	canonical.FactID = ""
	canonical.DeviceID = ""
	canonical.RecordedAt = time.Time{}
	canonical.DerivedPending = false
	raw, err := jsoncodec.Marshal(canonical)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	p.Receipt.PayloadHash = hex.EncodeToString(sum[:])
	return p, nil
}

func (s *Service) ConfirmMobileQuote(ctx context.Context, p *MobileQuotePlan) (*domain.MobileQuoteReceipt, error) {
	if p == nil {
		return nil, &domain.Error{Code: domain.ErrStalePreview, Field: "planId"}
	}
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	repo, ok := s.repository.(MobileQuoteRepository)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable}
	}
	old, err := repo.LookupMobileQuote(ctx, p.Receipt.HouseholdID, p.Receipt.OperationID)
	if err != nil {
		return nil, err
	}
	if old != nil {
		if old.PayloadHash != p.Receipt.PayloadHash {
			return nil, &domain.Error{Code: domain.ErrConflict, Field: "operationId"}
		}
		return s.maintainMobileQuote(ctx, old), nil
	}
	if time.Now().After(p.ExpiresAt) || p.token != s.writes.previewToken() {
		return nil, &domain.Error{Code: domain.ErrStalePreview, Field: "previewToken"}
	}
	p.Receipt.RecordedAt = s.clock()
	if p.instrument != nil {
		p.instrument.CreatedAt = p.Receipt.RecordedAt
	}
	if p.fx != nil {
		p.fx.CreatedAt = p.Receipt.RecordedAt
	}
	if err := repo.CommitMobileQuote(ctx, p.Receipt, p.instrument, p.fx); err != nil {
		return nil, err
	}
	s.invalidateAnalysis()
	return s.maintainMobileQuote(ctx, &p.Receipt), nil
}

func (s *Service) maintainMobileQuote(ctx context.Context, m *domain.MobileQuoteReceipt) *domain.MobileQuoteReceipt {
	out := *m
	if out.Kind == "fx" {
		out.DerivedPending = s.repriceMetalsForFX(ctx, out.HouseholdID, domain.CurrencyCode(out.Currency), domain.CurrencyCode(out.QuoteCurrency)) != nil
	}
	s.invalidateAnalysis()
	return &out
}

func (s *Service) LookupMobileQuote(ctx context.Context, id string) (*domain.MobileQuoteReceipt, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, mobileInvalid("operationId")
	}
	hh, err := s.requireHousehold(ctx)
	if err != nil {
		return nil, err
	}
	repo, ok := s.repository.(MobileQuoteRepository)
	if !ok {
		return nil, &domain.Error{Code: domain.ErrUnavailable}
	}
	return repo.LookupMobileQuote(ctx, hh.ID, id)
}
