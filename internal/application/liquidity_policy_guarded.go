package application

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// LiquidityPolicyPreview contains effective GUI policies, including assumed
// defaults after reset. Revision zero is a default, never a persisted policy.
type LiquidityPolicyPreview struct {
	Before       domain.LiquidityPolicy
	After        domain.LiquidityPolicy
	BeforeOrigin domain.PolicyOrigin
	AfterOrigin  domain.PolicyOrigin
}

func (s *Service) ordinaryLiquiditySource(ctx context.Context, ref domain.LiquiditySourceRef) (domain.LiquiditySource, error) {
	if err := ref.Validate(); err != nil {
		return domain.LiquiditySource{}, err
	}
	if ref.HoldingID != nil {
		product, err := s.repository.ProductByHolding(ctx, *ref.HoldingID)
		if err != nil {
			return domain.LiquiditySource{}, err
		}
		if product != nil {
			return domain.LiquiditySource{}, &domain.Error{Code: domain.ErrManagedPosition, Field: "source", Message: "managed product policy is updated with contract terms"}
		}
	}
	snapshot, err := s.repository.ReadLiquiditySnapshot(ctx)
	if err != nil {
		return domain.LiquiditySource{}, err
	}
	sources, err := s.liquiditySources(snapshot)
	if err != nil {
		return domain.LiquiditySource{}, err
	}
	for _, source := range sources {
		if source.Ref.Key() != ref.Key() || source.AccountID != ref.AccountID {
			continue
		}
		if source.Archived || source.Liability {
			return domain.LiquiditySource{}, &domain.Error{Code: domain.ErrValidation, Field: "source", Message: "requires an active asset source"}
		}
		if ref.Currency != nil && *ref.Currency != source.NativeCurrency {
			return domain.LiquiditySource{}, &domain.Error{Code: domain.ErrValidation, Field: "currency", Message: "must match the source currency"}
		}
		return source, nil
	}
	return domain.LiquiditySource{}, &domain.Error{Code: domain.ErrNotFound, Field: "source", Message: "active liquidity source was not found"}
}

func (s *Service) prepareOrdinaryPolicy(ctx context.Context, input SavePolicyInput, reset bool) (LiquidityPolicyPreview, error) {
	source, err := s.ordinaryLiquiditySource(ctx, input.Source)
	if err != nil {
		return LiquidityPolicyPreview{}, err
	}
	before, origin, _, err := domain.ResolveLiquidityPolicy(source)
	if err != nil {
		return LiquidityPolicyPreview{}, err
	}
	result := LiquidityPolicyPreview{Before: before, BeforeOrigin: origin, AfterOrigin: domain.PolicyOriginExplicit}
	if origin == domain.PolicyOriginAssumed {
		result.Before.Source, result.Before.Revision = input.Source, 0
	}
	if reset {
		if _, err = s.prepareLiquidityPolicyReset(ctx, input.Source, input.ExpectedRevision); err != nil {
			return result, err
		}
		source.ExplicitPolicy = nil
		result.After, result.AfterOrigin, _, err = domain.ResolveLiquidityPolicy(source)
		result.After.Source, result.After.Revision = input.Source, 0
	} else {
		result.After, err = s.prepareLiquidityPolicy(ctx, input)
	}
	return result, err
}

func (s *Service) PreviewLiquidityPolicyGuarded(ctx context.Context, input SavePolicyInput, reset bool) (LiquidityPolicyPreview, string, error) {
	unlock, err := s.beginPreviewRead(ctx)
	if err != nil {
		return LiquidityPolicyPreview{}, "", err
	}
	defer unlock()
	preview, err := s.prepareOrdinaryPolicy(ctx, input, reset)
	return preview, s.writes.previewToken(), err
}

// Policy edits use conservative MCP receipts, not recoverable business keys.
// The coordinator invalidates every attempted commit, including reset/recreate
// ABA revisions, and restart/restore invalidate all uncommitted plans.
func (s *Service) CommitLiquidityPolicyGuarded(ctx context.Context, input SavePolicyInput, reset bool, token string) (domain.LiquidityPolicy, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	defer unlock()
	if token != s.writes.previewToken() {
		return domain.LiquidityPolicy{}, staleProductData()
	}
	if _, err := s.prepareOrdinaryPolicy(ctx, input, reset); err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if reset {
		err := s.ResetLiquidityPolicy(ctx, input.Source, input.ExpectedRevision)
		return domain.LiquidityPolicy{Source: input.Source}, err
	}
	return s.SaveLiquidityPolicy(ctx, input)
}
