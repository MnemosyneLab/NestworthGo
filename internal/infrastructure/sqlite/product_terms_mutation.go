package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const productTermsMutationPrefix = "product.terms-mutation."

func productTermsMutationKey(household domain.HouseholdID, id domain.ProductOperationID) string {
	return productTermsMutationPrefix + household.String() + "." + id.String()
}

func (r *Repository) LookupProductTermsMutation(ctx context.Context, household domain.HouseholdID, id domain.ProductOperationID) (*domain.ProductTermsMutation, error) {
	var raw []byte
	err := r.database.SQL.QueryRowContext(ctx, `SELECT value FROM app_configuration WHERE key=?`, productTermsMutationKey(household, id)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m domain.ProductTermsMutation
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if m.HouseholdID != household || m.ID != id {
		return nil, storedIntegrity("product", "terms receipt identity is inconsistent")
	}
	return &m, nil
}

// The private receipt and revision-checked financial terms share one transaction.
// No standalone configuration writer may create this business receipt.
func (r *Repository) CommitProductTermsMutation(ctx context.Context, contract domain.ProductContract, policy domain.LiquidityPolicy, expectedPolicyRevision int, m domain.ProductTermsMutation) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.HouseholdID != contract.HouseholdID || m.ProductID != contract.ID {
		return storedIntegrity("product", "terms receipt ownership is inconsistent")
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := saveProductTermsTx(ctx, tx, contract, policy, expectedPolicyRevision); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_configuration(key,value) VALUES(?,?)`, productTermsMutationKey(m.HouseholdID, m.ID), string(raw)); err != nil {
			return err
		}
		return failProductCommit("terms:receipt")
	})
}

// Startup and read-only backup validation treat this private namespace as
// typed recovery evidence. JSON export intentionally excludes configuration.
func verifyProductTermsMutations(ctx context.Context, query schemaQuery) error {
	rows, err := query.QueryContext(ctx, `SELECT key,value FROM app_configuration WHERE substr(key,1,?)=?`, len(productTermsMutationPrefix), productTermsMutationPrefix)
	if err != nil {
		return err
	}
	defer rows.Close()
	var mutations []domain.ProductTermsMutation
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return err
		}
		var m domain.ProductTermsMutation
		if json.Unmarshal(raw, &m) != nil || m.Validate() != nil || key != productTermsMutationKey(m.HouseholdID, m.ID) {
			return storedIntegrity("product", "terms mutation evidence is invalid")
		}
		mutations = append(mutations, m)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(mutations) == 0 {
		return nil
	}
	contracts, err := query.QueryContext(ctx, productContractSelect)
	if err != nil {
		return err
	}
	defer contracts.Close()
	current := map[domain.ProductContractID]domain.ProductContract{}
	for contracts.Next() {
		c, err := scanProductContract(contracts)
		if err != nil {
			return err
		}
		current[c.ID] = c
	}
	if err := contracts.Err(); err != nil {
		return err
	}
	for _, m := range mutations {
		c, ok := current[m.ProductID]
		r := m.Receipt.Contract
		if !ok || c.Revision < r.Revision || c.HouseholdID != r.HouseholdID || c.AccountID != r.AccountID || c.HoldingID != r.HoldingID || c.InstrumentID != r.InstrumentID || c.Currency != r.Currency || c.Kind != r.Kind || c.StartOn != r.StartOn || c.OpenedOperationID != r.OpenedOperationID || !c.Principal.Amount().Equal(r.Principal.Amount()) {
			return storedIntegrity("product", "terms receipt does not belong to current facts")
		}
	}
	return nil
}
