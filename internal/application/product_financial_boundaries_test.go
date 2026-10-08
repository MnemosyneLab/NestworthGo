package application

import (
	"errors"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestProductInterestReceiptCannotMovePaidThroughBackwards(t *testing.T) {
	t.Parallel()
	s, ctx, account, opened := openRegressionProduct(t)
	detail, err := s.Product(ctx, opened.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateProductTerms(ctx, UpdateProductTermsInput{
		ProductID: detail.Contract.ID, ExpectedRevision: detail.Contract.Revision,
		Terms: ProductTermsInput{Kind: "term_deposit", Name: detail.Contract.Name, StartOn: detail.Contract.StartOn,
			MaturityOn: detail.Contract.MaturityOn, InterestMode: "simple_act_365",
			AnnualRatePercent: strPtr("3.65"), InterestPaidThroughOn: strPtr(detail.Contract.StartOn)},
		Policy: depositPolicy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.setClock(func() time.Time { return time.Date(2026, 9, 25, 5, 0, 0, 0, time.UTC) })
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpReceiveInterest,
		ReceiveInterest: &ReceiveInterestCommand{ProductID: detail.Contract.ID.String(), Amount: "50", InterestPaidThroughOn: strPtr("2026-09-25")}})
	s.setClock(func() time.Time { return time.Date(2026, 9, 26, 5, 0, 0, 0, time.UTC) })
	command := ProductCommand{Kind: domain.ProductOpReceiveInterest,
		ReceiveInterest: &ReceiveInterestCommand{ProductID: detail.Contract.ID.String(), Amount: "10", InterestPaidThroughOn: strPtr("2026-09-22")}}
	_, err = s.PreviewProductOperation(ctx, command)
	var validation *domain.Error
	if !errors.As(err, &validation) || validation.Code != domain.ErrValidation || validation.Field != "interestPaidThroughOn" {
		t.Fatalf("backwards paid-through date must fail validation, got %v", err)
	}
	current, err := s.Product(ctx, detail.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Contract.InterestPaidThroughOn == nil || *current.Contract.InterestPaidThroughOn != "2026-09-25" {
		t.Fatal("invalid receipt changed the paid-through date")
	}
	assertCash(t, s, ctx, account, "50050")
	// A separate actual payment can cover the same period without adding that
	// already-paid period back into the forecast.
	command.ReceiveInterest.InterestPaidThroughOn = strPtr("2026-09-25")
	recordRegressionProduct(t, s, ctx, command)
	assertCash(t, s, ctx, account, "50060")
}

func TestProductLongHistoryKeepsPaginationAndFinancialTimeChecks(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	s, ctx := newProductTestService(t, now)
	account := seedHoldingsCash(t, s, ctx, "20000")
	s.setClock(func() time.Time { return now.Add(time.Hour) })
	opened := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "10000", EffectiveAt: now.Add(time.Hour).Format(time.RFC3339),
		Terms:  ProductTermsInput{Kind: "locked_product", Name: "Long-lived product", StartOn: "2026-09-20", InterestMode: "none"},
		Policy: depositPolicy(),
	}})
	for index := 0; index < 101; index++ {
		observed := now.Add(2*time.Hour + time.Duration(index)*time.Minute)
		s.setClock(func() time.Time { return observed })
		_, err := s.AppendProductValuation(ctx, AppendProductValuationInput{
			ProductID: opened.ProductIDs[0], Amount: "10000", ObservedAt: observed.Format(time.RFC3339), MutationID: domain.NewProductOperationID().String(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, pageSize := range []struct {
		name  string
		input int
		want  int
	}{{"default", 0, 25}, {"below_maximum", 99, 99}, {"maximum", 100, 100}, {"capped", 1000, 100}} {
		t.Run(pageSize.name, func(t *testing.T) {
			seen := make(map[domain.ProductOperationID]bool)
			cursor := ""
			for {
				page, err := s.ListProductOperations(ctx, opened.ProductIDs[0], cursor, pageSize.input)
				if err != nil {
					t.Fatal(err)
				}
				if cursor == "" && len(page.Operations) != pageSize.want {
					t.Fatalf("first page has %d operations; expected %d", len(page.Operations), pageSize.want)
				}
				for _, operation := range page.Operations {
					if seen[operation.ID] {
						t.Fatal("pagination repeated an operation")
					}
					seen[operation.ID] = true
				}
				if page.Next == nil {
					if len(seen) != 102 {
						t.Fatalf("pagination stopped after %d operations; expected 102", len(seen))
					}
					break
				}
				cursor = *page.Next
			}
		})
	}
	command := ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: &ReceiveInterestCommand{
		ProductID: opened.ProductIDs[0].String(), Amount: "10", EffectiveAt: now.Add(30 * time.Minute).Format(time.RFC3339),
	}}
	_, err := s.PreviewProductOperation(ctx, command)
	var invalidTime *domain.Error
	if !errors.As(err, &invalidTime) || invalidTime.Code != domain.ErrInvalidChangeTime {
		t.Fatalf("receipt before product opening must fail after many valuations, got %v", err)
	}
	assertCash(t, s, ctx, account, "10000")
	command.ReceiveInterest.EffectiveAt = s.clock().Format(time.RFC3339)
	recordRegressionProduct(t, s, ctx, command)
	assertCash(t, s, ctx, account, "10010")
}
