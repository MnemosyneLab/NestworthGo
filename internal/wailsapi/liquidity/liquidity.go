package liquidity

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

func (s *Service) Overview(ctx context.Context, request OverviewRequest) (LiquidityOverviewDTO, error) {
	overview, err := s.app.LiquidityOverview(ctx, application.LiquidityOverviewQuery{
		CustomHorizonOn: request.CustomHorizonOn, IncludeEarlyWithdrawal: request.IncludeEarlyWithdrawal,
	})
	if err != nil {
		return LiquidityOverviewDTO{}, apierror.Wrap(err)
	}
	return fromOverview(overview), nil
}

func (s *Service) Product(ctx context.Context, productID string) (ProductDetailDTO, error) {
	id, err := domain.ParseProductContractID(strings.TrimSpace(productID))
	if err != nil {
		return ProductDetailDTO{}, apierror.Wrap(err)
	}
	detail, err := s.app.Product(ctx, id)
	if err != nil {
		return ProductDetailDTO{}, apierror.Wrap(err)
	}
	return fromProductDetail(detail), nil
}

func (s *Service) ListProducts(ctx context.Context, request ListProductsRequest) ([]ProductDetailDTO, error) {
	var accountID *domain.AccountID
	if request.AccountID != nil && strings.TrimSpace(*request.AccountID) != "" {
		parsed, err := domain.ParseAccountID(*request.AccountID)
		if err != nil {
			return nil, apierror.Wrap(err)
		}
		accountID = &parsed
	}
	details, err := s.app.ListProducts(ctx, accountID, request.IncludeClosed)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	result := make([]ProductDetailDTO, 0, len(details))
	for _, detail := range details {
		result = append(result, fromProductDetail(detail))
	}
	return result, nil
}

func (s *Service) ListOperations(ctx context.Context, request ListOperationsRequest) (ProductOperationPageDTO, error) {
	productID, err := domain.ParseProductContractID(strings.TrimSpace(request.ProductID))
	if err != nil {
		return ProductOperationPageDTO{}, apierror.Wrap(err)
	}
	page, err := s.app.ListProductOperations(ctx, productID, request.Cursor, request.Limit)
	if err != nil {
		return ProductOperationPageDTO{}, apierror.Wrap(err)
	}
	dto := ProductOperationPageDTO{Operations: make([]ProductOperationDTO, 0, len(page.Operations)), Next: page.Next}
	for _, operation := range page.Operations {
		dto.Operations = append(dto.Operations, fromOperation(operation))
	}
	return dto, nil
}

func (s *Service) SavePolicy(ctx context.Context, request SavePolicyRequest) (PolicyDTO, error) {
	source, err := request.Source.toDomain()
	if err != nil {
		return PolicyDTO{}, apierror.Wrap(err)
	}
	policy, err := s.app.SaveLiquidityPolicy(ctx, application.SavePolicyInput{
		Source: source, ExpectedRevision: request.ExpectedRevision, Policy: request.Policy,
	})
	if err != nil {
		return PolicyDTO{}, apierror.Wrap(err)
	}
	return fromPolicy(policy), nil
}

func (s *Service) ResetPolicy(ctx context.Context, request ResetPolicyRequest) (PolicyDTO, error) {
	source, err := request.Source.toDomain()
	if err != nil {
		return PolicyDTO{}, apierror.Wrap(err)
	}
	if err := s.app.ResetLiquidityPolicy(ctx, source, request.ExpectedRevision); err != nil {
		return PolicyDTO{}, apierror.Wrap(err)
	}
	return PolicyDTO{SourceRef: fromSourceRef(source), Revision: 0}, nil
}

func (s *Service) SaveReservation(ctx context.Context, request SaveReservationRequest) (ReservationDTO, error) {
	source, err := request.Source.toDomain()
	if err != nil {
		return ReservationDTO{}, apierror.Wrap(err)
	}
	input := application.SaveReservationInput{
		Source: source, ExpectedRevision: request.ExpectedRevision, Label: request.Label, Amount: request.Amount,
	}
	if request.ID != nil && strings.TrimSpace(*request.ID) != "" {
		id, err := domain.ParseLiquidityReservationID(*request.ID)
		if err != nil {
			return ReservationDTO{}, apierror.Wrap(err)
		}
		input.ID = &id
	}
	reservation, err := s.app.SaveLiquidityReservation(ctx, input)
	if err != nil {
		return ReservationDTO{}, apierror.Wrap(err)
	}
	return fromReservation(reservation), nil
}

func (s *Service) ReleaseReservation(ctx context.Context, request ReleaseReservationRequest) (ReservationDTO, error) {
	id, err := domain.ParseLiquidityReservationID(strings.TrimSpace(request.ID))
	if err != nil {
		return ReservationDTO{}, apierror.Wrap(err)
	}
	reservation, err := s.app.ReleaseLiquidityReservation(ctx, id, request.ExpectedRevision)
	if err != nil {
		return ReservationDTO{}, apierror.Wrap(err)
	}
	return fromReservation(reservation), nil
}

func (s *Service) UpdateProductTerms(ctx context.Context, request UpdateProductTermsRequest) (ProductDetailDTO, error) {
	productID, err := domain.ParseProductContractID(strings.TrimSpace(request.ProductID))
	if err != nil {
		return ProductDetailDTO{}, apierror.Wrap(err)
	}
	detail, err := s.app.UpdateProductTerms(ctx, application.UpdateProductTermsInput{
		ProductID: productID, ExpectedRevision: request.ExpectedRevision, Terms: request.Terms, Policy: request.Policy,
	})
	if err != nil {
		return ProductDetailDTO{}, apierror.Wrap(err)
	}
	return fromProductDetail(detail), nil
}

func (s *Service) AppendProductValuation(ctx context.Context, request AppendProductValuationRequest) (ProductDetailDTO, error) {
	productID, err := domain.ParseProductContractID(strings.TrimSpace(request.ProductID))
	if err != nil {
		return ProductDetailDTO{}, apierror.Wrap(err)
	}
	detail, err := s.app.AppendProductValuation(ctx, application.AppendProductValuationInput{
		ProductID: productID, Amount: request.Amount, ObservedAt: request.ObservedAt, MutationID: request.MutationID,
	})
	if err != nil {
		return ProductDetailDTO{}, apierror.Wrap(err)
	}
	return fromProductDetail(detail), nil
}

func (s *Service) PreviewProductOperation(ctx context.Context, request ProductCommandRequest) (ProductOperationPreviewDTO, error) {
	command, err := request.toApplication()
	if err != nil {
		return ProductOperationPreviewDTO{}, apierror.Wrap(err)
	}
	preview, err := s.app.PreviewProductOperation(ctx, command)
	if err != nil {
		return ProductOperationPreviewDTO{}, apierror.Wrap(err)
	}
	return fromPreview(preview), nil
}

func (s *Service) RecordProductOperation(ctx context.Context, request RecordProductOperationRequest) (ProductOperationReceiptDTO, error) {
	command, err := request.Command.toApplication()
	if err != nil {
		return ProductOperationReceiptDTO{}, apierror.Wrap(err)
	}
	receipt, err := s.app.RecordProductOperation(ctx, command, request.MutationID, request.ReviewedStateHash)
	if err != nil {
		return ProductOperationReceiptDTO{}, apierror.Wrap(err)
	}
	return fromReceipt(receipt), nil
}

func (s *Service) ListReservations(ctx context.Context) ([]ReservationDTO, error) {
	reservations, err := s.app.ListLiquidityReservations(ctx)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	result := make([]ReservationDTO, 0, len(reservations))
	for _, r := range reservations {
		result = append(result, fromReservation(r))
	}
	return result, nil
}
