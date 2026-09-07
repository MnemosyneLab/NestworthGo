package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type rowScanner interface {
	Scan(...any) error
}

func (r *Repository) Activity(ctx context.Context, householdID domain.HouseholdID, activityID domain.ActivityID) (domain.Activity, error) {
	activity, err := activityFromScanner(r.database.SQL.QueryRowContext(ctx, `SELECT id, household_id, kind, reason, effective_at, effective_local_date, created_at, note, reverses_activity_id, correction_group_id, transaction_fx_rate FROM activities WHERE household_id = ? AND id = ?`, householdID.String(), activityID.String()))
	if err != nil {
		return domain.Activity{}, err
	}
	activities := []domain.Activity{activity}
	if err := hydrateActivities(ctx, r.database.SQL, activities); err != nil {
		return domain.Activity{}, err
	}
	return activities[0], nil
}

func activityFromScanner(scanner rowScanner) (domain.Activity, error) {
	var id, householdID, kind, reason, effectiveAt, effectiveLocalDate, createdAt string
	var note, reversesID, correctionID, rate sql.NullString
	if err := scanner.Scan(&id, &householdID, &kind, &reason, &effectiveAt, &effectiveLocalDate, &createdAt, &note, &reversesID, &correctionID, &rate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Activity{}, &domain.Error{Code: domain.ErrNotFound, Message: "activity was not found"}
		}
		return domain.Activity{}, err
	}
	parsedID, err := domain.ParseActivityID(id)
	if err != nil {
		return domain.Activity{}, err
	}
	parsedHousehold, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.Activity{}, err
	}
	effective, err := time.Parse(time.RFC3339Nano, effectiveAt)
	if err != nil {
		return domain.Activity{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Activity{}, err
	}
	activity := domain.Activity{ID: parsedID, HouseholdID: parsedHousehold, Kind: domain.ActivityKind(kind), Reason: domain.ActivityReason(reason), EffectiveAt: effective.UTC(), EffectiveLocalDate: effectiveLocalDate, CreatedAt: created.UTC(), Note: parseNullable(nullString(note))}
	if reversesID.Valid && reversesID.String != "" {
		value, parseErr := domain.ParseActivityID(reversesID.String)
		if parseErr != nil {
			return domain.Activity{}, parseErr
		}
		activity.ReversesActivityID = &value
	}
	if correctionID.Valid && correctionID.String != "" {
		value, parseErr := domain.ParseActivityCorrectionGroupID(correctionID.String)
		if parseErr != nil {
			return domain.Activity{}, parseErr
		}
		activity.CorrectionGroupID = &value
	}
	if rate.Valid && rate.String != "" {
		value, parseErr := domain.ParseFxRate(rate.String)
		if parseErr != nil {
			return domain.Activity{}, parseErr
		}
		activity.TransactionFXRate = &value
	}
	return activity, nil
}

func (r *Repository) ActivityEffects(ctx context.Context, activityID domain.ActivityID) ([]domain.ActivityEffect, error) {
	return r.activityEffects(ctx, activityID)
}

func (r *Repository) ActivityHasReversal(ctx context.Context, householdID domain.HouseholdID, activityID domain.ActivityID) (bool, error) {
	var count int
	if err := r.database.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities WHERE household_id = ? AND reverses_activity_id = ?`, householdID.String(), activityID.String()).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) activityEffects(ctx context.Context, activityID domain.ActivityID) ([]domain.ActivityEffect, error) {
	return activityEffectsQuery(ctx, r.database.SQL, activityID)
}

func activityEffectsQuery(ctx context.Context, query queryer, activityID domain.ActivityID) ([]domain.ActivityEffect, error) {
	rows, err := query.QueryContext(ctx, `SELECT id, activity_id, sequence, role, direction, target, classification, account_id, holding_id, instrument_id, amount, currency, quantity, cost_unit_price FROM activity_effects WHERE activity_id = ? ORDER BY sequence ASC`, activityID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var effects []domain.ActivityEffect
	for rows.Next() {
		var id, activityValue, role, direction, target, classification string
		var sequence int
		var accountID, holdingID, instrumentID, amount, currency, quantity, costUnitPrice sql.NullString
		if err := rows.Scan(&id, &activityValue, &sequence, &role, &direction, &target, &classification, &accountID, &holdingID, &instrumentID, &amount, &currency, &quantity, &costUnitPrice); err != nil {
			return nil, err
		}
		effect, parseErr := scanActivityEffectWithSequence(id, activityValue, sequence, role, direction, target, classification, accountID, holdingID, instrumentID, amount, currency, quantity, costUnitPrice)
		if parseErr != nil {
			return nil, parseErr
		}
		effects = append(effects, effect)
	}
	return effects, rows.Err()
}

func scanActivityEffectWithSequence(id, activityID string, sequence int, role, direction, target, classification string, accountID, holdingID, instrumentID, amount, currency, quantity, costUnitPrice sql.NullString) (domain.ActivityEffect, error) {
	effectID, err := domain.ParseActivityEffectID(id)
	if err != nil {
		return domain.ActivityEffect{}, err
	}
	parsedActivity, err := domain.ParseActivityID(activityID)
	if err != nil {
		return domain.ActivityEffect{}, err
	}
	effect := domain.ActivityEffect{ID: effectID, ActivityID: parsedActivity, Sequence: sequence, Role: domain.EffectRole(role), Direction: domain.EffectDirection(direction), Target: domain.EffectTarget(target), Classification: domain.ActivityClassification(classification)}
	if accountID.Valid {
		value, parseErr := domain.ParseAccountID(accountID.String)
		if parseErr != nil {
			return domain.ActivityEffect{}, parseErr
		}
		effect.AccountID = &value
	}
	if holdingID.Valid {
		value, parseErr := domain.ParseHoldingID(holdingID.String)
		if parseErr != nil {
			return domain.ActivityEffect{}, parseErr
		}
		effect.HoldingID = &value
	}
	if instrumentID.Valid {
		value, parseErr := domain.ParseInstrumentID(instrumentID.String)
		if parseErr != nil {
			return domain.ActivityEffect{}, parseErr
		}
		effect.InstrumentID = &value
	}
	if amount.Valid {
		value, parseErr := domain.ParseMoney(amount.String, domain.CurrencyCode(currency.String))
		if parseErr != nil {
			return domain.ActivityEffect{}, parseErr
		}
		effect.Money = &value
	}
	if quantity.Valid {
		value, parseErr := domain.ParseQuantity(quantity.String)
		if parseErr != nil {
			return domain.ActivityEffect{}, parseErr
		}
		effect.Quantity = &value
	}
	if costUnitPrice.Valid {
		value, parseErr := domain.ParseUnitPrice(costUnitPrice.String)
		if parseErr != nil {
			return domain.ActivityEffect{}, parseErr
		}
		effect.CostUnitPrice = &value
	}
	return effect, nil
}

func (r *Repository) ListActivities(ctx context.Context, householdID domain.HouseholdID, limit int) ([]domain.Activity, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.database.SQL.QueryContext(ctx, `SELECT id, household_id, kind, reason, effective_at, effective_local_date, created_at, note, reverses_activity_id, correction_group_id, transaction_fx_rate FROM activities WHERE household_id = ? ORDER BY effective_at DESC, created_at DESC, id DESC LIMIT ?`, householdID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var activities []domain.Activity
	for rows.Next() {
		activity, scanErr := activityFromScanner(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := hydrateActivities(ctx, r.database.SQL, activities); err != nil {
		return nil, err
	}
	return activities, nil
}

func (r *Repository) ListActivityPage(ctx context.Context, householdID domain.HouseholdID, query domain.ActivityQuery) (domain.ActivityPage, error) {
	return listActivityPageQuery(ctx, r.database.SQL, householdID, query)
}

func listActivityPageQuery(ctx context.Context, db queryer, householdID domain.HouseholdID, query domain.ActivityQuery) (domain.ActivityPage, error) {
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	statement := `SELECT id, household_id, kind, reason, effective_at, effective_local_date, created_at, note, reverses_activity_id, correction_group_id, transaction_fx_rate FROM activities WHERE household_id = ?`
	args := []any{householdID.String()}
	if query.AccountID != nil {
		statement += ` AND EXISTS (SELECT 1 FROM activity_effects filter_effect WHERE filter_effect.activity_id = activities.id AND filter_effect.account_id = ?)`
		args = append(args, query.AccountID.String())
	}
	if query.InstrumentID != nil {
		statement += ` AND (EXISTS (SELECT 1 FROM activity_effects filter_effect WHERE filter_effect.activity_id = activities.id AND filter_effect.instrument_id = ?) OR EXISTS (SELECT 1 FROM activity_trade_details filter_trade WHERE filter_trade.activity_id = activities.id AND filter_trade.instrument_id = ?) OR EXISTS (SELECT 1 FROM activity_dividend_details filter_dividend WHERE filter_dividend.activity_id = activities.id AND filter_dividend.instrument_id = ?))`
		args = append(args, query.InstrumentID.String(), query.InstrumentID.String(), query.InstrumentID.String())
	}
	if len(query.Kinds) > 0 {
		placeholders := make([]string, len(query.Kinds))
		for index, kind := range query.Kinds {
			placeholders[index] = "?"
			args = append(args, kind.String())
		}
		statement += ` AND kind IN (` + strings.Join(placeholders, ",") + `)`
	}
	if query.ExcludeReversed {
		statement += ` AND reverses_activity_id IS NULL AND NOT EXISTS (SELECT 1 FROM activities reversal WHERE reversal.reverses_activity_id = activities.id)`
	}
	if query.FromLocalDate != "" {
		statement += ` AND effective_local_date >= ?`
		args = append(args, query.FromLocalDate)
	}
	if query.ToLocalDate != "" {
		statement += ` AND effective_local_date <= ?`
		args = append(args, query.ToLocalDate)
	}
	if query.After != nil {
		statement += ` AND (effective_at < ? OR (effective_at = ? AND (created_at < ? OR (created_at = ? AND id < ?))))`
		formattedEffective := formatTimestamp(query.After.EffectiveAt)
		formattedCreated := formatTimestamp(query.After.CreatedAt)
		args = append(args, formattedEffective, formattedEffective, formattedCreated, formattedCreated, query.After.ID.String())
	}
	statement += ` ORDER BY effective_at DESC, created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return domain.ActivityPage{}, err
	}
	defer rows.Close()
	var activities []domain.Activity
	for rows.Next() {
		activity, scanErr := activityFromScanner(rows)
		if scanErr != nil {
			return domain.ActivityPage{}, scanErr
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return domain.ActivityPage{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.ActivityPage{}, err
	}
	hasMore := len(activities) > limit
	if hasMore {
		activities = activities[:limit]
	}
	if err := hydrateActivities(ctx, db, activities); err != nil {
		return domain.ActivityPage{}, err
	}
	page := domain.ActivityPage{Activities: activities, HasMore: hasMore}
	if hasMore && len(activities) > 0 {
		last := activities[len(activities)-1]
		page.Next = &domain.ActivityCursor{EffectiveAt: last.EffectiveAt, CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func (r *Repository) ListActivitiesUntil(ctx context.Context, householdID domain.HouseholdID, cutoff time.Time) ([]domain.Activity, error) {
	return listActivitiesUntilQuery(ctx, r.database.SQL, householdID, cutoff)
}

func listActivitiesUntilQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, cutoff time.Time) ([]domain.Activity, error) {
	rows, err := query.QueryContext(ctx, `SELECT id, household_id, kind, reason, effective_at, effective_local_date, created_at, note, reverses_activity_id, correction_group_id, transaction_fx_rate FROM activities WHERE household_id = ? AND effective_at <= ? ORDER BY effective_at ASC, created_at ASC, id ASC`, householdID.String(), formatTimestamp(cutoff))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var activities []domain.Activity
	for rows.Next() {
		activity, scanErr := activityFromScanner(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := attachActivityEffects(ctx, query, activities); err != nil {
		return nil, err
	}
	return activities, nil
}

func hydrateActivities(ctx context.Context, query queryer, activities []domain.Activity) error {
	if err := attachActivityEffects(ctx, query, activities); err != nil {
		return err
	}
	if err := attachActivityTradeDetails(ctx, query, activities); err != nil {
		return err
	}
	if err := attachActivityDividendDetails(ctx, query, activities); err != nil {
		return err
	}
	return attachActivityResulting(ctx, query, activities)
}

func activityIDs(activities []domain.Activity) []string {
	ids := make([]string, len(activities))
	for index := range activities {
		ids[index] = activities[index].ID.String()
	}
	return ids
}

func activityIndexByID(activities []domain.Activity) map[string]int {
	indexByID := make(map[string]int, len(activities))
	for index := range activities {
		indexByID[activities[index].ID.String()] = index
	}
	return indexByID
}

func attachActivityEffects(ctx context.Context, query queryer, activities []domain.Activity) error {
	if len(activities) == 0 {
		return nil
	}
	clause, args := sqlInArgs(activityIDs(activities))
	rows, err := query.QueryContext(ctx, `SELECT id, activity_id, sequence, role, direction, target, classification, account_id, holding_id, instrument_id, amount, currency, quantity, cost_unit_price FROM activity_effects WHERE activity_id IN (`+clause+`) ORDER BY activity_id, sequence ASC`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	indexByID := activityIndexByID(activities)
	for index := range activities {
		activities[index].Effects = nil
	}
	for rows.Next() {
		var id, activityValue, role, direction, target, classification string
		var sequence int
		var accountID, holdingID, instrumentID, amount, currency, quantity, costUnitPrice sql.NullString
		if err := rows.Scan(&id, &activityValue, &sequence, &role, &direction, &target, &classification, &accountID, &holdingID, &instrumentID, &amount, &currency, &quantity, &costUnitPrice); err != nil {
			return err
		}
		effect, parseErr := scanActivityEffectWithSequence(id, activityValue, sequence, role, direction, target, classification, accountID, holdingID, instrumentID, amount, currency, quantity, costUnitPrice)
		if parseErr != nil {
			return parseErr
		}
		index, ok := indexByID[activityValue]
		if !ok {
			continue
		}
		activities[index].Effects = append(activities[index].Effects, effect)
	}
	return rows.Err()
}

func attachActivityTradeDetails(ctx context.Context, query queryer, activities []domain.Activity) error {
	if len(activities) == 0 {
		return nil
	}
	clause, args := sqlInArgs(activityIDs(activities))
	rows, err := query.QueryContext(ctx, `SELECT activity_id, side, instrument_id, holding_id, quantity, gross_amount, gross_currency, unit_price, fee_amount, fee_currency FROM activity_trade_details WHERE activity_id IN (`+clause+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	indexByID := activityIndexByID(activities)
	for rows.Next() {
		var activityID, side, instrumentID, holdingID, quantity, grossAmount, grossCurrency, unitPrice string
		var feeAmount, feeCurrency sql.NullString
		if err := rows.Scan(&activityID, &side, &instrumentID, &holdingID, &quantity, &grossAmount, &grossCurrency, &unitPrice, &feeAmount, &feeCurrency); err != nil {
			return err
		}
		detail, parseErr := parseTradeDetail(side, instrumentID, holdingID, quantity, grossAmount, grossCurrency, unitPrice, feeAmount, feeCurrency)
		if parseErr != nil {
			return parseErr
		}
		index, ok := indexByID[activityID]
		if !ok {
			continue
		}
		activities[index].TradeDetail = detail
	}
	return rows.Err()
}

func attachActivityDividendDetails(ctx context.Context, query queryer, activities []domain.Activity) error {
	if len(activities) == 0 {
		return nil
	}
	clause, args := sqlInArgs(activityIDs(activities))
	rows, err := query.QueryContext(ctx, `SELECT activity_id, holding_id, instrument_id, amount, currency FROM activity_dividend_details WHERE activity_id IN (`+clause+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	indexByID := activityIndexByID(activities)
	for rows.Next() {
		var activityID, holdingID, instrumentID, amount, currency string
		if err := rows.Scan(&activityID, &holdingID, &instrumentID, &amount, &currency); err != nil {
			return err
		}
		detail, parseErr := parseDividendDetail(holdingID, instrumentID, amount, currency)
		if parseErr != nil {
			return parseErr
		}
		index, ok := indexByID[activityID]
		if !ok {
			continue
		}
		activities[index].DividendDetail = detail
	}
	return rows.Err()
}

func attachActivityResulting(ctx context.Context, query queryer, activities []domain.Activity) error {
	if len(activities) == 0 {
		return nil
	}
	clause, args := sqlInArgs(activityIDs(activities))
	unionArgs := make([]any, 0, len(args)*3)
	unionArgs = append(unionArgs, args...)
	unionArgs = append(unionArgs, args...)
	unionArgs = append(unionArgs, args...)
	rows, err := query.QueryContext(ctx, `SELECT e.activity_id, e.target, e.account_id, e.holding_id, av.amount, av.currency, ''
FROM activity_effects e
JOIN account_values av ON av.activity_effect_id = e.id AND av.projection_kind = 'event'
WHERE e.activity_id IN (`+clause+`)
UNION ALL
SELECT e.activity_id, e.target, e.account_id, e.holding_id, ac.amount, ac.currency, ''
FROM activity_effects e
JOIN account_cash_values ac ON ac.activity_effect_id = e.id AND ac.projection_kind = 'event'
WHERE e.activity_id IN (`+clause+`)
UNION ALL
SELECT e.activity_id, e.target, e.account_id, e.holding_id, '', '', hq.quantity
FROM activity_effects e
JOIN holding_quantity_values hq ON hq.activity_effect_id = e.id AND hq.projection_kind = 'event'
WHERE e.activity_id IN (`+clause+`)`, unionArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	indexByID := activityIndexByID(activities)
	for rows.Next() {
		var activityID, target string
		var accountID, holdingID, amount, currency, quantity sql.NullString
		if err := rows.Scan(&activityID, &target, &accountID, &holdingID, &amount, &currency, &quantity); err != nil {
			return err
		}
		view, parseErr := parseEndpointView(target, accountID, holdingID, amount, currency, quantity)
		if parseErr != nil {
			return parseErr
		}
		index, ok := indexByID[activityID]
		if !ok {
			continue
		}
		activities[index].Resulting = append(activities[index].Resulting, view)
	}
	return rows.Err()
}

func parseTradeDetail(side, instrumentID, holdingID, quantity, grossAmount, grossCurrency, unitPrice string, feeAmount, feeCurrency sql.NullString) (*domain.TradeDetail, error) {
	parsedSide, err := domain.ParseTradeSide(side)
	if err != nil {
		return nil, err
	}
	parsedInstrument, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return nil, err
	}
	parsedHolding, err := domain.ParseHoldingID(holdingID)
	if err != nil {
		return nil, err
	}
	parsedQuantity, err := domain.ParseQuantity(quantity)
	if err != nil {
		return nil, err
	}
	gross, err := domain.ParseMoney(grossAmount, domain.CurrencyCode(grossCurrency))
	if err != nil {
		return nil, err
	}
	parsedPrice, err := domain.ParseUnitPrice(unitPrice)
	if err != nil {
		return nil, err
	}
	detail := &domain.TradeDetail{Side: parsedSide, InstrumentID: parsedInstrument, HoldingID: parsedHolding, Quantity: parsedQuantity, Gross: gross, UnitPrice: parsedPrice}
	if feeAmount.Valid && feeAmount.String != "" {
		fee, parseErr := domain.ParseMoney(feeAmount.String, domain.CurrencyCode(feeCurrency.String))
		if parseErr != nil {
			return nil, parseErr
		}
		detail.Fee = &fee
	}
	return detail, nil
}

func parseDividendDetail(holdingID, instrumentID, amount, currency string) (*domain.DividendDetail, error) {
	parsedHolding, err := domain.ParseHoldingID(holdingID)
	if err != nil {
		return nil, err
	}
	parsedInstrument, err := domain.ParseInstrumentID(instrumentID)
	if err != nil {
		return nil, err
	}
	parsedAmount, err := domain.ParseMoney(amount, domain.CurrencyCode(currency))
	if err != nil {
		return nil, err
	}
	return &domain.DividendDetail{HoldingID: parsedHolding, InstrumentID: parsedInstrument, Amount: parsedAmount}, nil
}

func parseEndpointView(target string, accountID, holdingID, amount, currency, quantity sql.NullString) (domain.EndpointView, error) {
	view := domain.EndpointView{Target: domain.EffectTarget(target), Amount: amount.String, Quantity: quantity.String, Currency: domain.CurrencyCode(currency.String)}
	if accountID.Valid && accountID.String != "" {
		parsed, parseErr := domain.ParseAccountID(accountID.String)
		if parseErr != nil {
			return domain.EndpointView{}, parseErr
		}
		view.AccountID = &parsed
	}
	if holdingID.Valid && holdingID.String != "" {
		parsed, parseErr := domain.ParseHoldingID(holdingID.String)
		if parseErr != nil {
			return domain.EndpointView{}, parseErr
		}
		view.HoldingID = &parsed
	}
	return view, nil
}
