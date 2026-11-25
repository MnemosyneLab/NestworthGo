package history

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type AmendmentLinks struct {
	Original           domain.ActivityID
	CorrectionReversal domain.ActivityID
	Reversal           domain.ActivityID
	Replacement        domain.ActivityID
}

func loadAmendmentLinks(ctx context.Context, reader Reader, activity domain.Activity) (AmendmentLinks, error) {
	var links AmendmentLinks
	if activity.ReversesActivityID != nil {
		links.Original = *activity.ReversesActivityID
		if activity.CorrectionGroupID != nil {
			_, replacement, err := correctionPair(ctx, reader, *activity.CorrectionGroupID)
			if err != nil {
				return AmendmentLinks{}, err
			}
			links.Replacement = replacement.ID
		}
		return links, nil
	}
	if activity.CorrectionGroupID != nil {
		reversal, _, err := correctionPair(ctx, reader, *activity.CorrectionGroupID)
		if err != nil {
			return AmendmentLinks{}, err
		}
		links.Original, links.CorrectionReversal = *reversal.ReversesActivityID, reversal.ID
	}
	page, err := reader.ListActivityPage(ctx, domain.ActivityQuery{ReversesActivityID: &activity.ID, Limit: 1})
	if err != nil {
		return AmendmentLinks{}, err
	}
	if len(page.Activities) == 0 {
		return links, nil
	}
	reversal := page.Activities[0]
	links.Reversal = reversal.ID
	if reversal.CorrectionGroupID != nil {
		_, replacement, err := correctionPair(ctx, reader, *reversal.CorrectionGroupID)
		if err != nil {
			return AmendmentLinks{}, err
		}
		links.Replacement = replacement.ID
	}
	return links, nil
}

func correctionPair(ctx context.Context, reader Reader, group domain.ActivityCorrectionGroupID) (domain.Activity, domain.Activity, error) {
	page, err := reader.ListActivityPage(ctx, domain.ActivityQuery{CorrectionGroupID: &group, Limit: 2})
	if err != nil {
		return domain.Activity{}, domain.Activity{}, err
	}
	var reversal, replacement domain.Activity
	for _, activity := range page.Activities {
		if activity.ReversesActivityID == nil {
			replacement = activity
		} else {
			reversal = activity
		}
	}
	if page.HasMore || reversal.ID == "" || replacement.ID == "" {
		return domain.Activity{}, domain.Activity{}, &domain.Error{Code: domain.ErrIntegrity, Message: "incomplete correction pair"}
	}
	return reversal, replacement, nil
}
