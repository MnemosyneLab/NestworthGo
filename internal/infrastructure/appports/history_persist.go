package appports

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type sqliteHistoryPersist struct {
	repo *sqlite.Repository
}

// AttachSQLiteHistory wires historical batch persistence. sqlite stays
// independent of application types; this adapter is the only translation.
func AttachSQLiteHistory(service *application.Service, repo *sqlite.Repository) {
	if service == nil || repo == nil {
		return
	}
	service.SetHistoryPersister(sqliteHistoryPersist{repo: repo})
}

func (p sqliteHistoryPersist) PersistInstrumentHistory(ctx context.Context, request application.CommitInstrumentHistoryRequest) (application.CommitHistoryResult, error) {
	result, err := p.repo.CommitInstrumentHistory(ctx, instrumentHistoryCommitFromRequest(request))
	if err != nil {
		return application.CommitHistoryResult{}, err
	}
	return historyCommitResultFromSQLite(result), nil
}

func (p sqliteHistoryPersist) PersistFXHistory(ctx context.Context, request application.CommitFXHistoryRequest) (application.CommitHistoryResult, error) {
	result, err := p.repo.CommitFXHistory(ctx, fxHistoryCommitFromRequest(request))
	if err != nil {
		return application.CommitHistoryResult{}, err
	}
	return historyCommitResultFromSQLite(result), nil
}

func instrumentHistoryCommitFromRequest(request application.CommitInstrumentHistoryRequest) sqlite.InstrumentHistoryCommit {
	commit := sqlite.InstrumentHistoryCommit{
		HouseholdID:    request.HouseholdID,
		InstrumentID:   request.InstrumentID,
		ProviderKey:    request.Identity.ProviderKey,
		ProviderSymbol: request.Identity.ProviderSymbol,
		QuoteCurrency:  request.Identity.QuoteCurrency,
		Market:         request.Market,
		Status:         string(request.Outcome.Status),
		Reason:         request.Outcome.Reason,
		Adapter:        request.Outcome.Batch.Evidence.Adapter,
		SourcePolicy:   application.SourcePolicyVersion(request.Outcome.Batch.Evidence),
		FetchedAt:      request.FetchedAt,
		NextCheckAt:    request.Outcome.Batch.NextCheckAt,
	}
	for _, observation := range request.Outcome.Batch.Observations {
		commit.Observations = append(commit.Observations, sqlite.InstrumentHistoryObservation{
			MarketDate:        string(observation.MarketDate),
			Value:             observation.Value,
			Currency:          observation.Currency,
			ValueEffectiveAt:  observation.ValueEffectiveAt,
			ProviderTimestamp: observation.ProviderTimestamp,
			Kind:              string(observation.Kind),
			PriceBasis:        string(observation.PriceBasis),
			TimestampBasis:    string(observation.TimestampBasis),
			SplitFactor:       observation.SplitFactor,
			DividendCash:      observation.DividendCash,
		})
	}
	for _, rng := range request.Outcome.Batch.VerifiedRanges {
		commit.VerifiedRanges = append(commit.VerifiedRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	for _, rng := range request.Outcome.Batch.PendingRanges {
		commit.PendingRanges = append(commit.PendingRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	return commit
}

func fxHistoryCommitFromRequest(request application.CommitFXHistoryRequest) sqlite.FXHistoryCommit {
	commit := sqlite.FXHistoryCommit{
		HouseholdID:   request.HouseholdID,
		ProviderKey:   request.Identity.ProviderKey,
		BaseCurrency:  request.Identity.BaseCurrency,
		QuoteCurrency: request.Identity.QuoteCurrency,
		Status:        string(request.Outcome.Status),
		Reason:        request.Outcome.Reason,
		Adapter:       request.Outcome.Batch.Evidence.Adapter,
		SourcePolicy:  application.SourcePolicyVersion(request.Outcome.Batch.Evidence),
		FetchedAt:     request.FetchedAt,
		NextCheckAt:   request.Outcome.Batch.NextCheckAt,
	}
	for _, observation := range request.Outcome.Batch.Observations {
		commit.Observations = append(commit.Observations, sqlite.FXHistoryObservation{
			MarketDate:       string(observation.MarketDate),
			Rate:             observation.Rate,
			BaseCurrency:     observation.BaseCurrency,
			QuoteCurrency:    observation.QuoteCurrency,
			ValueEffectiveAt: observation.ValueEffectiveAt,
			Kind:             string(observation.Kind),
			TimestampBasis:   string(observation.TimestampBasis),
		})
	}
	for _, rng := range request.Outcome.Batch.VerifiedRanges {
		commit.VerifiedRanges = append(commit.VerifiedRanges, sqlite.DateSpan{Start: string(rng.Start), End: string(rng.End)})
	}
	return commit
}

func historyCommitResultFromSQLite(result sqlite.HistoryCommitResult) application.CommitHistoryResult {
	return application.CommitHistoryResult{
		PersistedObservations: result.PersistedObservations,
		NewRevisions:          result.NewRevisions,
		CoverageDays:          result.CoverageDays,
		CanonicalSlots:        result.CanonicalSlots,
		InputGeneration:       result.InputGeneration,
		Unchanged:             result.Unchanged,
	}
}
