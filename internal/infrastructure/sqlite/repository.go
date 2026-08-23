package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type Repository struct{ database *DB }

func NewRepository(database *DB) *Repository { return &Repository{database: database} }

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *Repository) DB() *DB { return r.database }

func (r *Repository) ReadSnapshot(ctx context.Context, filter domain.AccountFilter) (domain.ReadSnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ReadSnapshot{}, err
	}
	household, err := scanHousehold(tx.QueryRowContext(ctx, `SELECT id, name, base_currency, created_at, updated_at FROM households WHERE singleton_key = 1`))
	if err != nil {
		_ = tx.Rollback()
		return domain.ReadSnapshot{}, err
	}
	if household == nil {
		if err := tx.Commit(); err != nil {
			return domain.ReadSnapshot{}, err
		}
		return domain.ReadSnapshot{}, nil
	}
	members, err := listMembersQuery(ctx, tx, true)
	if err != nil {
		_ = tx.Rollback()
		return domain.ReadSnapshot{}, err
	}
	institutions, err := listInstitutionsQuery(ctx, tx, true)
	if err != nil {
		_ = tx.Rollback()
		return domain.ReadSnapshot{}, err
	}
	groups, err := listGroupsQuery(ctx, tx, true)
	if err != nil {
		_ = tx.Rollback()
		return domain.ReadSnapshot{}, err
	}
	accounts, err := listAccountRecords(ctx, tx, household.ID, filter)
	if err != nil {
		_ = tx.Rollback()
		return domain.ReadSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ReadSnapshot{}, err
	}
	return domain.ReadSnapshot{Household: household, Members: members, Institutions: institutions, Groups: groups, Accounts: accounts}, nil
}

func listMembersQuery(ctx context.Context, query queryer, includeArchived bool) ([]domain.Member, error) {
	statement := `SELECT id, household_id, name, avatar_asset_id, note, sort_order, created_at, updated_at, archived_at FROM members`
	if !includeArchived {
		statement += ` WHERE archived_at IS NULL`
	}
	statement += ` ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`
	rows, err := query.QueryContext(ctx, statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Member
	for rows.Next() {
		member, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, member)
	}
	return result, rows.Err()
}

func listInstitutionsQuery(ctx context.Context, query queryer, includeArchived bool) ([]domain.Institution, error) {
	statement := `SELECT id, household_id, name, icon_key, institution_type, country_code, website, note, logo_asset_id, sort_order, created_at, updated_at, archived_at FROM institutions`
	if !includeArchived {
		statement += ` WHERE archived_at IS NULL`
	}
	statement += ` ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`
	rows, err := query.QueryContext(ctx, statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Institution
	for rows.Next() {
		item, err := scanInstitution(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func listGroupsQuery(ctx context.Context, query queryer, includeArchived bool) ([]domain.Group, error) {
	statement := `SELECT id, household_id, name, icon_key, color, logo_asset_id, description, sort_order, created_at, updated_at, archived_at FROM account_groups`
	if !includeArchived {
		statement += ` WHERE archived_at IS NULL`
	}
	statement += ` ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`
	rows, err := query.QueryContext(ctx, statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Group
	for rows.Next() {
		item, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (r *Repository) Household(ctx context.Context) (*domain.Household, error) {
	row := r.database.SQL.QueryRowContext(ctx, `SELECT id, name, base_currency, created_at, updated_at FROM households WHERE singleton_key = 1`)
	return scanHousehold(row)
}

func (r *Repository) CreateOnboarding(ctx context.Context, household domain.Household, members []domain.Member) error {
	if len(members) == 0 {
		return errors.New("at least one member is required")
	}
	operation := func(tx *sql.Tx) error {
		var existing string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM households WHERE singleton_key = 1`).Scan(&existing); err == nil {
			return errors.New("household already exists")
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO households(id, singleton_key, name, base_currency, created_at, updated_at) VALUES(?, 1, ?, ?, ?, ?)`, household.ID.String(), household.Name, household.BaseCurrency.String(), formatTimestamp(household.CreatedAt), formatTimestamp(household.UpdatedAt)); err != nil {
			return err
		}
		for index, member := range members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO members(id, household_id, name, note, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, member.ID.String(), household.ID.String(), member.Name, nullableString(member.Note), index, formatTimestamp(member.CreatedAt), formatTimestamp(member.UpdatedAt)); err != nil {
				return err
			}
		}
		return nil
	}
	return r.database.WithTx(ctx, operation)
}

func (r *Repository) ListMembers(ctx context.Context, includeArchived bool) ([]domain.Member, error) {
	query := `SELECT id, household_id, name, avatar_asset_id, note, sort_order, created_at, updated_at, archived_at FROM members`
	if !includeArchived {
		query += ` WHERE archived_at IS NULL`
	}
	query += ` ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`
	rows, err := r.database.SQL.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Member
	for rows.Next() {
		member, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, member)
	}
	return result, rows.Err()
}

// CreateMember assigns sort_order with a MAX(sort_order)+1 subquery evaluated
// inside the same write transaction as the INSERT. Because the database is
// opened with a single connection and _txlock=immediate, write transactions
// are fully serialized, so two concurrent CreateMember calls can never
// compute and insert the same sort_order (see docs/development/code-review-2026-08-21.md BUG-6).
func (r *Repository) CreateMember(ctx context.Context, member domain.Member) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO members(id, household_id, name, note, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM members WHERE household_id = ?), ?, ?)`, member.ID.String(), member.HouseholdID.String(), member.Name, nullableString(member.Note), member.HouseholdID.String(), formatTimestamp(member.CreatedAt), formatTimestamp(member.UpdatedAt))
		return err
	})
}
func (r *Repository) UpdateMember(ctx context.Context, member domain.Member) error {
	result, err := r.database.SQL.ExecContext(ctx, `UPDATE members SET name = ?, note = ?, updated_at = ? WHERE id = ? AND household_id = ?`, member.Name, nullableString(member.Note), formatTimestamp(member.UpdatedAt), member.ID.String(), member.HouseholdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "member")
}

func (r *Repository) SetMemberArchive(ctx context.Context, householdID domain.HouseholdID, id domain.MemberID, archived bool, now time.Time) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if archived {
			var active int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM members WHERE household_id = ? AND archived_at IS NULL AND id <> ?`, householdID.String(), id.String()).Scan(&active); err != nil {
				return err
			}
			if active < 1 {
				return &domain.Error{Code: domain.ErrConflict, Message: "household must retain at least one active member"}
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE members SET archived_at = ?, updated_at = ? WHERE id = ? AND household_id = ?`, archiveValue(archived, now), formatTimestamp(now), id.String(), householdID.String())
		if err != nil {
			return err
		}
		return requireAffected(result, "member")
	})
}

// CreateInstitution computes sort_order the same race-free way as
// CreateMember; see the comment there.
func (r *Repository) CreateInstitution(ctx context.Context, institution domain.Institution) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO institutions(id, household_id, name, icon_key, institution_type, country_code, website, note, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM institutions WHERE household_id = ?), ?, ?)`, institution.ID.String(), institution.HouseholdID.String(), institution.Name, nullableString(institution.IconKey), nullableString(institution.InstitutionType), nullableString(institution.CountryCode), nullableString(institution.Website), nullableString(institution.Note), institution.HouseholdID.String(), formatTimestamp(institution.CreatedAt), formatTimestamp(institution.UpdatedAt))
		return err
	})
}
func (r *Repository) UpdateInstitution(ctx context.Context, institution domain.Institution) error {
	result, err := r.database.SQL.ExecContext(ctx, `UPDATE institutions SET name = ?, icon_key = ?, institution_type = ?, country_code = ?, website = ?, note = ?, updated_at = ? WHERE id = ? AND household_id = ?`, institution.Name, nullableString(institution.IconKey), nullableString(institution.InstitutionType), nullableString(institution.CountryCode), nullableString(institution.Website), nullableString(institution.Note), formatTimestamp(institution.UpdatedAt), institution.ID.String(), institution.HouseholdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "institution")
}

func (r *Repository) ListInstitutions(ctx context.Context, includeArchived bool) ([]domain.Institution, error) {
	query := `SELECT id, household_id, name, icon_key, institution_type, country_code, website, note, logo_asset_id, sort_order, created_at, updated_at, archived_at FROM institutions`
	if !includeArchived {
		query += ` WHERE archived_at IS NULL`
	}
	query += ` ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`
	rows, err := r.database.SQL.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Institution
	for rows.Next() {
		institution, err := scanInstitution(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, institution)
	}
	return result, rows.Err()
}

func (r *Repository) SetInstitutionArchive(ctx context.Context, householdID domain.HouseholdID, id domain.InstitutionID, archived bool, now time.Time) error {
	return r.setArchive(ctx, "institutions", householdID.String(), id.String(), archived, now)
}

// CreateGroup computes sort_order the same race-free way as CreateMember;
// see the comment there.
func (r *Repository) CreateGroup(ctx context.Context, group domain.Group) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO account_groups(id, household_id, name, icon_key, color, description, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM account_groups WHERE household_id = ?), ?, ?)`, group.ID.String(), group.HouseholdID.String(), group.Name, nullableString(group.IconKey), nullableString(group.Color), nullableString(group.Description), group.HouseholdID.String(), formatTimestamp(group.CreatedAt), formatTimestamp(group.UpdatedAt))
		return err
	})
}
func (r *Repository) UpdateGroup(ctx context.Context, group domain.Group) error {
	result, err := r.database.SQL.ExecContext(ctx, `UPDATE account_groups SET name = ?, icon_key = ?, color = ?, description = ?, updated_at = ? WHERE id = ? AND household_id = ?`, group.Name, nullableString(group.IconKey), nullableString(group.Color), nullableString(group.Description), formatTimestamp(group.UpdatedAt), group.ID.String(), group.HouseholdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "group")
}

func (r *Repository) ListGroups(ctx context.Context, includeArchived bool) ([]domain.Group, error) {
	query := `SELECT id, household_id, name, icon_key, color, logo_asset_id, description, sort_order, created_at, updated_at, archived_at FROM account_groups`
	if !includeArchived {
		query += ` WHERE archived_at IS NULL`
	}
	query += ` ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`
	rows, err := r.database.SQL.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Group
	for rows.Next() {
		group, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, group)
	}
	return result, rows.Err()
}

func (r *Repository) CreateMediaAsset(ctx context.Context, asset domain.MediaAsset) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO media_assets(id, household_id, mime_type, data, created_at) VALUES(?, ?, ?, ?, ?)`, asset.ID.String(), asset.HouseholdID.String(), asset.MimeType, asset.Data, formatTimestamp(asset.CreatedAt))
		return err
	})
}
func (r *Repository) MediaAsset(ctx context.Context, householdID domain.HouseholdID, id domain.MediaAssetID) (domain.MediaAsset, error) {
	var rawID, rawHousehold, mimeType, createdAt string
	var data []byte
	if err := r.database.SQL.QueryRowContext(ctx, `SELECT id, household_id, mime_type, data, created_at FROM media_assets WHERE id = ? AND household_id = ?`, id.String(), householdID.String()).Scan(&rawID, &rawHousehold, &mimeType, &data, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.MediaAsset{}, &domain.Error{Code: domain.ErrNotFound, Message: "media asset was not found"}
		}
		return domain.MediaAsset{}, err
	}
	parsedID, err := domain.ParseMediaAssetID(rawID)
	if err != nil {
		return domain.MediaAsset{}, err
	}
	parsedHousehold, err := domain.ParseHouseholdID(rawHousehold)
	if err != nil {
		return domain.MediaAsset{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.MediaAsset{}, err
	}
	return domain.MediaAsset{ID: parsedID, HouseholdID: parsedHousehold, MimeType: mimeType, Data: append([]byte(nil), data...), CreatedAt: created.UTC()}, nil
}

func (r *Repository) SetMemberAvatar(ctx context.Context, householdID domain.HouseholdID, memberID domain.MemberID, assetID domain.MediaAssetID) error {
	return r.setMediaReference(ctx, "members", "avatar_asset_id", householdID.String(), memberID.String(), assetID.String())
}
func (r *Repository) SetInstitutionLogo(ctx context.Context, householdID domain.HouseholdID, id domain.InstitutionID, assetID domain.MediaAssetID) error {
	return r.setMediaReference(ctx, "institutions", "logo_asset_id", householdID.String(), id.String(), assetID.String())
}
func (r *Repository) SetGroupLogo(ctx context.Context, householdID domain.HouseholdID, id domain.GroupID, assetID domain.MediaAssetID) error {
	return r.setMediaReference(ctx, "account_groups", "logo_asset_id", householdID.String(), id.String(), assetID.String())
}
func (r *Repository) SetAccountLogo(ctx context.Context, householdID domain.HouseholdID, id domain.AccountID, assetID domain.MediaAssetID) error {
	return r.setMediaReference(ctx, "accounts", "logo_asset_id", householdID.String(), id.String(), assetID.String())
}

func (r *Repository) SetInstitutionIcon(ctx context.Context, householdID domain.HouseholdID, id domain.InstitutionID, iconKey string) error {
	return r.setIconReference(ctx, "institutions", householdID.String(), id.String(), iconKey)
}

func (r *Repository) SetGroupIcon(ctx context.Context, householdID domain.HouseholdID, id domain.GroupID, iconKey string) error {
	return r.setIconReference(ctx, "account_groups", householdID.String(), id.String(), iconKey)
}

func (r *Repository) SetAccountIcon(ctx context.Context, householdID domain.HouseholdID, id domain.AccountID, iconKey string) error {
	return r.setIconReference(ctx, "accounts", householdID.String(), id.String(), iconKey)
}

func (r *Repository) setMediaReference(ctx context.Context, table, column, householdID, id, assetID string) error {
	allowed := (table == "members" && column == "avatar_asset_id") || (table == "institutions" && column == "logo_asset_id") || (table == "account_groups" && column == "logo_asset_id") || (table == "accounts" && column == "logo_asset_id")
	if !allowed {
		return errors.New("unsupported media reference")
	}
	query := fmt.Sprintf(`UPDATE %s SET %s = ?, updated_at = ? WHERE id = ? AND household_id = ?`, table, column)
	result, err := r.database.SQL.ExecContext(ctx, query, assetID, formatTimestamp(time.Now()), id, householdID)
	if err != nil {
		return err
	}
	return requireAffected(result, table)
}

func (r *Repository) setIconReference(ctx context.Context, table, householdID, id, iconKey string) error {
	if table != "institutions" && table != "account_groups" && table != "accounts" {
		return errors.New("unsupported icon reference")
	}
	query := fmt.Sprintf(`UPDATE %s SET icon_key = ?, updated_at = ? WHERE id = ? AND household_id = ?`, table)
	result, err := r.database.SQL.ExecContext(ctx, query, iconKey, formatTimestamp(time.Now()), id, householdID)
	if err != nil {
		return err
	}
	return requireAffected(result, table)
}

func (r *Repository) SetGroupArchive(ctx context.Context, householdID domain.HouseholdID, id domain.GroupID, archived bool, now time.Time) error {
	return r.setArchive(ctx, "account_groups", householdID.String(), id.String(), archived, now)
}

func (r *Repository) setArchive(ctx context.Context, table, householdID, id string, archived bool, now time.Time) error {
	if table != "institutions" && table != "account_groups" {
		return errors.New("unsupported archive table")
	}
	query := fmt.Sprintf(`UPDATE %s SET archived_at = ?, updated_at = ? WHERE id = ? AND household_id = ?`, table)
	result, err := r.database.SQL.ExecContext(ctx, query, archiveValue(archived, now), formatTimestamp(now), id, householdID)
	if err != nil {
		return err
	}
	return requireAffected(result, table)
}

func (r *Repository) CreateAccount(ctx context.Context, account domain.Account, ownership domain.Ownership, initial *domain.AccountValue) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := validateAccountReferences(ctx, tx, account, nil, nil); err != nil {
			return err
		}
		if account.TrackingMode == domain.TrackingHoldings && initial != nil {
			return &domain.Error{Code: domain.ErrValidation, Field: "initialValue", Message: "Holdings accounts cannot have an initial Account Value"}
		}
		if account.TrackingMode != domain.TrackingHoldings && initial == nil {
			return &domain.Error{Code: domain.ErrValidation, Field: "initialValue", Message: "an initial Account Value is required"}
		}
		if initial != nil && (initial.AccountID != account.ID || initial.ValueKind != account.TrackingMode || initial.Amount.Currency() != account.DefaultCurrency) {
			return &domain.Error{Code: domain.ErrValidation, Field: "initialValue", Message: "initial value does not match the account"}
		}
		if err := insertAccount(ctx, tx, account); err != nil {
			return err
		}
		if err := replaceOwnership(ctx, tx, account.ID, ownership, false); err != nil {
			return err
		}
		if initial == nil {
			return nil
		}
		return insertAccountValue(ctx, tx, *initial)
	})
}

func (r *Repository) UpdateAccount(ctx context.Context, account domain.Account, ownership domain.Ownership) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var currentInstitution, currentGroup sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT institution_id, group_id FROM accounts WHERE id = ? AND household_id = ?`, account.ID.String(), account.HouseholdID.String()).Scan(&currentInstitution, &currentGroup); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "account was not found"}
			}
			return err
		}
		if err := validateAccountReferences(ctx, tx, account, nullableStringValue(currentInstitution), nullableStringValue(currentGroup)); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE accounts SET institution_id = ?, group_id = ?, name = ?, primary_category = ?, secondary_category = ?, tracking_mode = ?, default_currency = ?, note = ?, icon_key = ?, logo_asset_id = ?, include_in_net_worth = ?, include_in_investment = ?, include_in_liquid_assets = ?, opened_on = ?, closed_on = ?, sort_order = ?, updated_at = ? WHERE id = ? AND household_id = ?`, nullableID(account.InstitutionID), nullableID(account.GroupID), account.Name, account.PrimaryCategory.String(), string(account.SecondaryCategory), string(account.TrackingMode), account.DefaultCurrency.String(), nullableString(account.Note), nullableString(account.IconKey), nullableID(account.LogoAssetID), boolValue(account.IncludeInNetWorth), boolValue(account.IncludeInInvestment), boolValue(account.IncludeInLiquidAssets), nullableString(account.OpenedOn), nullableString(account.ClosedOn), account.SortOrder, formatTimestamp(account.UpdatedAt), account.ID.String(), account.HouseholdID.String())
		if err != nil {
			return err
		}
		if err := requireAffected(result, "account"); err != nil {
			return err
		}
		return replaceOwnership(ctx, tx, account.ID, ownership, true)
	})
}

func (r *Repository) AppendAccountValue(ctx context.Context, value domain.AccountValue) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error { return insertAccountValue(ctx, tx, value) })
}

func (r *Repository) SetAccountArchive(ctx context.Context, householdID domain.HouseholdID, id domain.AccountID, archived bool, now time.Time) error {
	result, err := r.database.SQL.ExecContext(ctx, `UPDATE accounts SET archived_at = ?, updated_at = ? WHERE id = ? AND household_id = ?`, archiveValue(archived, now), formatTimestamp(now), id.String(), householdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "account")
}

func (r *Repository) ListAccountRecords(ctx context.Context, householdID domain.HouseholdID, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	records, err := listAccountRecords(ctx, tx, householdID, filter)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return records, nil
}

func listAccountRecords(ctx context.Context, query queryer, householdID domain.HouseholdID, filter domain.AccountFilter) ([]domain.AccountRecord, error) {
	where := []string{"a.household_id = ?"}
	args := []any{householdID.String()}
	if !filter.IncludeArchived {
		where = append(where, "a.archived_at IS NULL")
	}
	if filter.MemberID != nil {
		where = append(where, "EXISTS (SELECT 1 FROM account_ownership f_ao WHERE f_ao.account_id = a.id AND f_ao.member_id = ?)")
		args = append(args, filter.MemberID.String())
	}
	if filter.InstitutionID != nil {
		where = append(where, "a.institution_id = ?")
		args = append(args, filter.InstitutionID.String())
	}
	if filter.GroupID != nil {
		where = append(where, "a.group_id = ?")
		args = append(args, filter.GroupID.String())
	}
	if filter.Category != nil {
		where = append(where, "a.primary_category = ?")
		args = append(args, filter.Category.String())
	}
	switch filter.OwnershipScope {
	case domain.OwnershipSole:
		where = append(where, "(SELECT COUNT(*) FROM account_ownership scope_ao WHERE scope_ao.account_id = a.id) = 1")
	case domain.OwnershipShared:
		where = append(where, "(SELECT COUNT(*) FROM account_ownership scope_ao WHERE scope_ao.account_id = a.id) > 1")
	}
	queryText := `SELECT a.id, a.household_id, a.institution_id, a.group_id, a.name, a.primary_category, a.secondary_category, a.tracking_mode, a.default_currency, a.note, a.icon_key, a.logo_asset_id, a.include_in_net_worth, a.include_in_investment, a.include_in_liquid_assets, a.opened_on, a.closed_on, a.sort_order, a.created_at, a.updated_at, a.archived_at, COALESCE(i.name, ''), COALESCE(g.name, '') FROM accounts a LEFT JOIN institutions i ON i.id = a.institution_id LEFT JOIN account_groups g ON g.id = a.group_id WHERE ` + strings.Join(where, " AND ") + ` ORDER BY a.sort_order ASC, a.name COLLATE NOCASE ASC, a.id ASC`
	rows, err := query.QueryContext(ctx, queryText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]domain.AccountRecord, 0)
	byID := make(map[domain.AccountID]int)
	for rows.Next() {
		record, err := scanAccountRecord(rows)
		if err != nil {
			return nil, err
		}
		byID[record.Account.ID] = len(accounts)
		accounts = append(accounts, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return accounts, nil
	}
	owners, err := loadOwnership(ctx, query, householdID)
	if err != nil {
		return nil, err
	}
	for id, shares := range owners {
		if index, ok := byID[id]; ok {
			parsed, parseErr := domain.ParseOwnership(shares)
			if parseErr != nil {
				return nil, parseErr
			}
			accounts[index].Ownership = parsed
		}
	}
	values, err := loadLatestValues(ctx, query, householdID)
	if err != nil {
		return nil, err
	}
	for id, value := range values {
		if index, ok := byID[id]; ok {
			accounts[index].LatestValue = value
		}
	}
	return accounts, nil
}

func loadOwnership(ctx context.Context, query queryer, householdID domain.HouseholdID) (map[domain.AccountID][]domain.OwnershipShare, error) {
	rows, err := query.QueryContext(ctx, `SELECT ao.account_id, ao.member_id, ao.share_bps FROM account_ownership ao JOIN accounts a ON a.id = ao.account_id WHERE a.household_id = ? ORDER BY ao.account_id, ao.member_id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[domain.AccountID][]domain.OwnershipShare)
	for rows.Next() {
		var accountID, memberID string
		var share int
		if err := rows.Scan(&accountID, &memberID, &share); err != nil {
			return nil, err
		}
		parsedAccount, err := domain.ParseAccountID(accountID)
		if err != nil {
			return nil, err
		}
		parsedMember, err := domain.ParseMemberID(memberID)
		if err != nil {
			return nil, err
		}
		result[parsedAccount] = append(result[parsedAccount], domain.OwnershipShare{MemberID: parsedMember, ShareBPS: share})
	}
	return result, rows.Err()
}

func loadLatestValues(ctx context.Context, query queryer, householdID domain.HouseholdID) (map[domain.AccountID]*domain.AccountValue, error) {
	rows, err := query.QueryContext(ctx, `SELECT av.id, av.account_id, av.value_kind, av.amount, av.currency, av.effective_at, av.created_at FROM account_values av JOIN accounts a ON a.id = av.account_id WHERE a.household_id = ? AND NOT EXISTS (SELECT 1 FROM account_values newer WHERE newer.account_id = av.account_id AND (newer.effective_at > av.effective_at OR (newer.effective_at = av.effective_at AND newer.created_at > av.created_at) OR (newer.effective_at = av.effective_at AND newer.created_at = av.created_at AND newer.id > av.id)))`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[domain.AccountID]*domain.AccountValue)
	for rows.Next() {
		var id, accountID, valueKind, amount, currency, effectiveAt, createdAt string
		if err := rows.Scan(&id, &accountID, &valueKind, &amount, &currency, &effectiveAt, &createdAt); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseAccountValueID(id)
		if err != nil {
			return nil, err
		}
		parsedAccountID, err := domain.ParseAccountID(accountID)
		if err != nil {
			return nil, err
		}
		mode, err := domain.ParseTrackingMode(valueKind)
		if err != nil {
			return nil, err
		}
		parsedCurrency, err := domain.ParseCurrency(currency)
		if err != nil {
			return nil, err
		}
		money, err := domain.ParseMoney(amount, parsedCurrency)
		if err != nil {
			return nil, err
		}
		effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
		if err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		result[parsedAccountID] = &domain.AccountValue{ID: parsedID, AccountID: parsedAccountID, ValueKind: mode, Amount: money, EffectiveAt: effective.UTC(), CreatedAt: created.UTC()}
	}
	return result, rows.Err()
}

func validateAccountReferences(ctx context.Context, tx *sql.Tx, account domain.Account, retainedInstitution, retainedGroup *string) error {
	check := func(table, id string, retained *string) error {
		var householdID string
		var archived sql.NullString
		query := fmt.Sprintf("SELECT household_id, archived_at FROM %s WHERE id = ?", table)
		if err := tx.QueryRowContext(ctx, query, id).Scan(&householdID, &archived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: table + " reference was not found"}
			}
			return err
		}
		if householdID != account.HouseholdID.String() {
			return &domain.Error{Code: domain.ErrValidation, Field: table, Message: "reference belongs to another household"}
		}
		if archived.Valid && (retained == nil || *retained != id) {
			return &domain.Error{Code: domain.ErrValidation, Field: table, Message: "reference is archived"}
		}
		return nil
	}
	if account.InstitutionID != nil {
		if err := check("institutions", account.InstitutionID.String(), retainedInstitution); err != nil {
			return err
		}
	}
	if account.GroupID != nil {
		if err := check("account_groups", account.GroupID.String(), retainedGroup); err != nil {
			return err
		}
	}
	return nil
}

func insertAccount(ctx context.Context, tx *sql.Tx, account domain.Account) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO accounts(id, household_id, institution_id, group_id, name, primary_category, secondary_category, tracking_mode, default_currency, note, icon_key, logo_asset_id, include_in_net_worth, include_in_investment, include_in_liquid_assets, opened_on, closed_on, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, account.ID.String(), account.HouseholdID.String(), nullableID(account.InstitutionID), nullableID(account.GroupID), account.Name, account.PrimaryCategory.String(), string(account.SecondaryCategory), string(account.TrackingMode), account.DefaultCurrency.String(), nullableString(account.Note), nullableString(account.IconKey), nullableID(account.LogoAssetID), boolValue(account.IncludeInNetWorth), boolValue(account.IncludeInInvestment), boolValue(account.IncludeInLiquidAssets), nullableString(account.OpenedOn), nullableString(account.ClosedOn), account.SortOrder, formatTimestamp(account.CreatedAt), formatTimestamp(account.UpdatedAt))
	return err
}

func replaceOwnership(ctx context.Context, tx *sql.Tx, accountID domain.AccountID, ownership domain.Ownership, allowArchivedExisting bool) error {
	var householdID string
	if err := tx.QueryRowContext(ctx, `SELECT household_id FROM accounts WHERE id = ?`, accountID.String()).Scan(&householdID); err != nil {
		return err
	}
	existing := make(map[string]bool)
	rows, err := tx.QueryContext(ctx, `SELECT member_id FROM account_ownership WHERE account_id = ?`, accountID.String())
	if err != nil {
		return err
	}
	for rows.Next() {
		var memberID string
		if err := rows.Scan(&memberID); err != nil {
			_ = rows.Close()
			return err
		}
		existing[memberID] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_ownership WHERE account_id = ?`, accountID.String()); err != nil {
		return err
	}
	for _, share := range ownership.Shares() {
		var memberHousehold string
		var archived sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT household_id, archived_at FROM members WHERE id = ?`, share.MemberID.String()).Scan(&memberHousehold, &archived); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "member was not found"}
			}
			return err
		}
		if memberHousehold != householdID {
			return &domain.Error{Code: domain.ErrValidation, Field: "ownership", Message: "owner belongs to another household"}
		}
		if archived.Valid && (!allowArchivedExisting || !existing[share.MemberID.String()]) {
			return &domain.Error{Code: domain.ErrValidation, Field: "ownership", Message: "new owners must be active members"}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_ownership(account_id, member_id, share_bps) VALUES(?, ?, ?)`, accountID.String(), share.MemberID.String(), share.ShareBPS); err != nil {
			return err
		}
	}
	return nil
}

func insertAccountValue(ctx context.Context, tx *sql.Tx, value domain.AccountValue) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, value.ID.String(), value.AccountID.String(), string(value.ValueKind), value.Amount.CanonicalAmount(), value.Amount.Currency().String(), formatTimestamp(value.EffectiveAt), formatTimestamp(value.CreatedAt))
	return err
}

func scanHousehold(row interface{ Scan(...any) error }) (*domain.Household, error) {
	var id, name, currency, createdAt, updatedAt string
	if err := row.Scan(&id, &name, &currency, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	householdID, err := domain.ParseHouseholdID(id)
	if err != nil {
		return nil, err
	}
	baseCurrency, err := domain.ParseCurrency(currency)
	if err != nil {
		return nil, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, err
	}
	return &domain.Household{ID: householdID, Name: name, BaseCurrency: baseCurrency, CreatedAt: created.UTC(), UpdatedAt: updated.UTC()}, nil
}

func scanMember(row interface{ Scan(...any) error }) (domain.Member, error) {
	var id, householdID, name, createdAt, updatedAt string
	var avatar, note, archived sql.NullString
	var sortOrder int
	if err := row.Scan(&id, &householdID, &name, &avatar, &note, &sortOrder, &createdAt, &updatedAt, &archived); err != nil {
		return domain.Member{}, err
	}
	memberID, err := domain.ParseMemberID(id)
	if err != nil {
		return domain.Member{}, err
	}
	hID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.Member{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Member{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.Member{}, err
	}
	archivedAt, err := parseTimePtr(archived)
	if err != nil {
		return domain.Member{}, err
	}
	return domain.Member{ID: memberID, HouseholdID: hID, Name: name, AvatarAssetID: parseMediaID(nullString(avatar)), Note: parseNullable(nullString(note)), SortOrder: sortOrder, CreatedAt: created.UTC(), UpdatedAt: updated.UTC(), ArchivedAt: archivedAt}, nil
}

func scanInstitution(row interface{ Scan(...any) error }) (domain.Institution, error) {
	var id, householdID, name, createdAt, updatedAt string
	var icon, institutionType, country, website, note, logo, archived sql.NullString
	var sortOrder int
	if err := row.Scan(&id, &householdID, &name, &icon, &institutionType, &country, &website, &note, &logo, &sortOrder, &createdAt, &updatedAt, &archived); err != nil {
		return domain.Institution{}, err
	}
	institutionID, err := domain.ParseInstitutionID(id)
	if err != nil {
		return domain.Institution{}, err
	}
	hID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.Institution{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Institution{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.Institution{}, err
	}
	archivedAt, err := parseTimePtr(archived)
	if err != nil {
		return domain.Institution{}, err
	}
	return domain.Institution{ID: institutionID, HouseholdID: hID, Name: name, IconKey: parseNullable(nullString(icon)), InstitutionType: parseNullable(nullString(institutionType)), CountryCode: parseNullable(nullString(country)), Website: parseNullable(nullString(website)), Note: parseNullable(nullString(note)), LogoAssetID: parseMediaID(nullString(logo)), SortOrder: sortOrder, CreatedAt: created.UTC(), UpdatedAt: updated.UTC(), ArchivedAt: archivedAt}, nil
}

func scanGroup(row interface{ Scan(...any) error }) (domain.Group, error) {
	var id, householdID, name, createdAt, updatedAt string
	var icon, color, logo, description, archived sql.NullString
	var sortOrder int
	if err := row.Scan(&id, &householdID, &name, &icon, &color, &logo, &description, &sortOrder, &createdAt, &updatedAt, &archived); err != nil {
		return domain.Group{}, err
	}
	groupID, err := domain.ParseGroupID(id)
	if err != nil {
		return domain.Group{}, err
	}
	hID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.Group{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Group{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.Group{}, err
	}
	archivedAt, err := parseTimePtr(archived)
	if err != nil {
		return domain.Group{}, err
	}
	return domain.Group{ID: groupID, HouseholdID: hID, Name: name, IconKey: parseNullable(nullString(icon)), Color: parseNullable(nullString(color)), LogoAssetID: parseMediaID(nullString(logo)), Description: parseNullable(nullString(description)), SortOrder: sortOrder, CreatedAt: created.UTC(), UpdatedAt: updated.UTC(), ArchivedAt: archivedAt}, nil
}

func scanAccountRecord(row interface{ Scan(...any) error }) (domain.AccountRecord, error) {
	var id, householdID, name, primary, secondary, tracking, currency, createdAt, updatedAt, institutionName, groupName string
	var institution, group, note, icon, logo, opened, closed, archived sql.NullString
	var includeNetWorth, includeInvestment, includeLiquid, sortOrder int
	if err := row.Scan(&id, &householdID, &institution, &group, &name, &primary, &secondary, &tracking, &currency, &note, &icon, &logo, &includeNetWorth, &includeInvestment, &includeLiquid, &opened, &closed, &sortOrder, &createdAt, &updatedAt, &archived, &institutionName, &groupName); err != nil {
		return domain.AccountRecord{}, err
	}
	accountID, err := domain.ParseAccountID(id)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	hID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	category, err := domain.ParsePrimaryCategory(primary)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	secondaryCategory, err := domain.ParseSecondaryCategory(secondary)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	mode, err := domain.ParseTrackingMode(tracking)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	currencyCode, err := domain.ParseCurrency(currency)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	archivedAt, err := parseTimePtr(archived)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.AccountRecord{}, err
	}
	return domain.AccountRecord{Account: domain.Account{ID: accountID, HouseholdID: hID, InstitutionID: parseInstitutionID(nullString(institution)), GroupID: parseGroupID(nullString(group)), Name: name, PrimaryCategory: category, SecondaryCategory: secondaryCategory, TrackingMode: mode, DefaultCurrency: currencyCode, Note: parseNullable(nullString(note)), IconKey: parseNullable(nullString(icon)), LogoAssetID: parseMediaID(nullString(logo)), IncludeInNetWorth: includeNetWorth != 0, IncludeInInvestment: includeInvestment != 0, IncludeInLiquidAssets: includeLiquid != 0, OpenedOn: parseNullable(nullString(opened)), ClosedOn: parseNullable(nullString(closed)), SortOrder: sortOrder, CreatedAt: created.UTC(), UpdatedAt: updated.UTC(), ArchivedAt: archivedAt}, InstitutionName: institutionName, GroupName: groupName}, nil
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
func formatTimestamp(value time.Time) string {
	return value.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z07:00")
}
func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullableID[T ~string](value *T) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
func nullableStringValue(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
func nullableBool(value bool) int {
	if value {
		return 1
	}
	return 0
}
func boolValue(value bool) int { return nullableBool(value) }
func archiveValue(archived bool, now time.Time) any {
	if archived {
		return formatTimestamp(now)
	}
	return nil
}
func requireAffected(result sql.Result, entity string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return &domain.Error{Code: domain.ErrNotFound, Message: entity + " was not found"}
	}
	return nil
}

func parseNullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func parseTimePtr(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}
func parseMediaID(value string) *domain.MediaAssetID {
	if value == "" {
		return nil
	}
	id, err := domain.ParseMediaAssetID(value)
	if err != nil {
		return nil
	}
	return &id
}
func parseInstitutionID(value string) *domain.InstitutionID {
	if value == "" {
		return nil
	}
	id, err := domain.ParseInstitutionID(value)
	if err != nil {
		return nil
	}
	return &id
}
func parseGroupID(value string) *domain.GroupID {
	if value == "" {
		return nil
	}
	id, err := domain.ParseGroupID(value)
	if err != nil {
		return nil
	}
	return &id
}
