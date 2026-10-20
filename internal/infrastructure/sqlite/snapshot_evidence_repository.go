package sqlite

import (
	"context"
	"database/sql"
	"github.com/waltwang/nestworth-go/internal/domain"
	"sort"
	"strings"
)

// ReadSnapshotQuoteEvidence hydrates only identities referenced by selected
// snapshots. Each bounded IN query shares one read transaction; unrelated
// activities, archived leaves and quote history never enter this read.
func (r *Repository) ReadSnapshotQuoteEvidence(ctx context.Context, household domain.HouseholdID, snapshots []domain.DailyValuationSnapshot) (domain.PortfolioSnapshot, error) {
	out := domain.PortfolioSnapshot{OriginInstrumentSources: map[domain.InstrumentID]domain.QuoteSourceKind{}, InstrumentPreferenceSources: map[domain.InstrumentPreferenceObservationID]domain.QuoteSourceKind{}}
	preferences := map[string]bool{}
	accounts, instruments, prices, fx := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, s := range snapshots {
		for _, i := range s.Items {
			accounts[i.AccountID.String()] = true
			if i.PreferenceObservationID != nil {
				preferences[i.PreferenceObservationID.String()] = true
			}
			if i.InstrumentID != nil {
				instruments[i.InstrumentID.String()] = true
			}
			if i.QuoteID != nil {
				prices[*i.QuoteID] = true
			}
			if i.FXQuoteID != nil {
				fx[*i.FXQuoteID] = true
			}
		}
	}
	if len(accounts) == 0 && len(prices) == 0 && len(fx) == 0 {
		return out, nil
	}
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	read := func(ids map[string]bool, statement string, scan func(*sql.Rows) error) error {
		keys := make([]string, 0, len(ids))
		for id := range ids {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for start := 0; start < len(keys); start += 256 {
			end := min(start+256, len(keys))
			args := []any{household.String()}
			for _, id := range keys[start:end] {
				args = append(args, id)
			}
			rows, err := tx.QueryContext(ctx, strings.Replace(statement, "{ids}", strings.TrimSuffix(strings.Repeat("?,", end-start), ","), 1), args...)
			if err != nil {
				return err
			}
			err = scan(rows)
			rows.Close()
			if err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
		}
		return nil
	}
	err = read(accounts, `SELECT id,balance_sheet_role,account_type,tracking_mode,default_currency FROM accounts WHERE household_id=? AND id IN ({ids})`, func(rows *sql.Rows) error {
		for rows.Next() {
			var a domain.Account
			if err := rows.Scan(&a.ID, &a.BalanceSheetRole, &a.AccountType, &a.TrackingMode, &a.DefaultCurrency); err != nil {
				return err
			}
			out.Accounts = append(out.Accounts, domain.AccountRecord{Account: a})
		}
		return rows.Err()
	})
	if err != nil {
		return out, err
	}
	err = read(instruments, `SELECT id,instrument_type,quote_currency,quote_source,quantity_unit FROM instruments WHERE household_id=? AND id IN ({ids})`, func(rows *sql.Rows) error {
		for rows.Next() {
			var i domain.Instrument
			if err := rows.Scan(&i.ID, &i.Type, &i.QuoteCurrency, &i.QuoteSource, &i.QuantityUnit); err != nil {
				return err
			}
			out.Instruments = append(out.Instruments, i)
		}
		return rows.Err()
	})
	if err != nil {
		return out, err
	}
	// Bind carry semantics to the snapshot's effective preference, never today's setting.
	err = read(instruments, `SELECT p.instrument_id,p.source_kind FROM history_origin_instrument_preferences p JOIN history_origins o ON o.id=p.origin_id WHERE o.household_id=? AND p.instrument_id IN ({ids})`, func(rows *sql.Rows) error {
		for rows.Next() {
			var id domain.InstrumentID
			var source domain.QuoteSourceKind
			if err := rows.Scan(&id, &source); err != nil {
				return err
			}
			out.OriginInstrumentSources[id] = source
		}
		return rows.Err()
	})
	if err != nil {
		return out, err
	}
	err = read(preferences, `SELECT p.id,p.source_kind FROM instrument_preference_observations p JOIN instruments i ON i.id=p.instrument_id WHERE i.household_id=? AND p.id IN ({ids})`, func(rows *sql.Rows) error {
		for rows.Next() {
			var id domain.InstrumentPreferenceObservationID
			var source domain.QuoteSourceKind
			if err := rows.Scan(&id, &source); err != nil {
				return err
			}
			out.InstrumentPreferenceSources[id] = source
		}
		return rows.Err()
	})
	if err != nil {
		return out, err
	}
	err = read(prices, `SELECT q.id,q.instrument_id,q.unit_price,q.currency,q.source_kind,q.source_key,q.quoted_at,q.created_at,q.delayed,q.observation_kind,q.effective_date,q.provider_timestamp,q.fetched_at,q.value_effective_at,q.binding_revision,q.source_policy_version,q.price_basis,q.timestamp_basis,q.revision,q.supersedes_quote_id,q.split_factor,q.dividend_cash,q.conversion_json FROM instrument_quotes q JOIN instruments i ON i.id=q.instrument_id WHERE i.household_id=? AND q.id IN ({ids}) AND NOT EXISTS(SELECT 1 FROM agent_quote_records withdrawn WHERE withdrawn.target_type='instrument' AND withdrawn.target_quote_id=q.id)`, func(rows *sql.Rows) error {
		quotes, err := scanInstrumentQuotes(rows)
		out.InstrumentQuotes = append(out.InstrumentQuotes, quotes...)
		return err
	})
	if err != nil {
		return out, err
	}
	err = read(fx, `SELECT id,household_id,base_currency,quote_currency,rate,source_kind,source_key,quoted_at,created_at,delayed,observation_kind,effective_date,fetched_at,value_effective_at,source_policy_version,timestamp_basis,revision,supersedes_quote_id FROM fx_quotes WHERE household_id=? AND id IN ({ids}) AND NOT EXISTS(SELECT 1 FROM agent_quote_records withdrawn WHERE withdrawn.target_type='fx' AND withdrawn.target_quote_id=fx_quotes.id)`, func(rows *sql.Rows) error {
		quotes, err := scanFXQuotes(rows)
		out.FXQuotes = append(out.FXQuotes, quotes...)
		return err
	})
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
