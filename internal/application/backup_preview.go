package application

import "context"

type BackupSidePreview struct {
	HouseholdName string
	BaseCurrency  string
	Accounts      int
	Holdings      int
	Activities    int
}

func (s *Service) CurrentDatabasePreview(ctx context.Context) (BackupSidePreview, error) {
	household, err := s.repository.Household(ctx)
	if err != nil {
		return BackupSidePreview{}, err
	}
	preview := BackupSidePreview{}
	if household == nil {
		return preview, nil
	}
	preview.HouseholdName = household.Name
	preview.BaseCurrency = household.BaseCurrency.String()
	accounts, holdings, activities, err := s.repository.PreviewCounts(ctx)
	if err != nil {
		return BackupSidePreview{}, err
	}
	preview.Accounts = accounts
	preview.Holdings = holdings
	preview.Activities = activities
	return preview, nil
}
