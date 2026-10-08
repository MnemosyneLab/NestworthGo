package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

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
		return nil, asStoredIntegrity("product", err)
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
	if same, err := sameProductTermsFacts(m.Receipt, contract, policy); err != nil {
		return err
	} else if !same {
		return storedIntegrity("product", "terms receipt does not match committed facts")
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

// Compare every persisted contract/policy field through the receipt's exact
// money-aware encoder, with timestamps projected to SQLite's UTC millisecond
// precision. Older sealed receipts may retain nanoseconds or an offset: never
// rewrite their original evidence or digest, nor discard any non-time field.
func sameProductTermsFacts(receipt domain.ProductTermsReceipt, contract domain.ProductContract, policy domain.LiquidityPolicy) (bool, error) {
	normalize := func(c *domain.ProductContract, p *domain.LiquidityPolicy) {
		c.CreatedAt, c.UpdatedAt = productFactTime(c.CreatedAt), productFactTime(c.UpdatedAt)
		p.CreatedAt, p.UpdatedAt = productFactTime(p.CreatedAt), productFactTime(p.UpdatedAt)
		if p.ConfirmedAt != nil {
			at := productFactTime(*p.ConfirmedAt)
			p.ConfirmedAt = &at
		}
	}
	normalize(&receipt.Contract, &receipt.Policy)
	normalize(&contract, &policy)
	expected := receipt
	expected.Contract = contract
	expected.Policy = policy
	a, err := json.Marshal(receipt)
	if err != nil {
		return false, err
	}
	b, err := json.Marshal(expected)
	return bytes.Equal(a, b), err
}

func productFactTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Millisecond)
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

	if err := contracts.Close(); err != nil {
		return err
	}
	households := map[domain.HouseholdID]bool{}
	for _, m := range mutations {
		households[m.HouseholdID] = true
	}
	policies := map[domain.LiquidityPolicyID]domain.LiquidityPolicy{}
	for household := range households {
		list, err := listLiquidityPoliciesQuery(ctx, query, household)
		if err != nil {
			return err
		}
		for _, p := range list {
			policies[p.ID] = p
		}
	}
	for _, m := range mutations {
		c, ok := current[m.ProductID]
		r := m.Receipt.Contract
		if !ok || c.Revision < r.Revision || c.HouseholdID != r.HouseholdID || c.AccountID != r.AccountID || c.HoldingID != r.HoldingID || c.InstrumentID != r.InstrumentID || c.Currency != r.Currency || c.Kind != r.Kind || c.StartOn != r.StartOn || c.OpenedOperationID != r.OpenedOperationID || !c.Principal.Amount().Equal(r.Principal.Amount()) {
			return storedIntegrity("product", "terms receipt does not belong to current facts")
		}

		policy, ok := policies[m.Receipt.Policy.ID]
		if !ok || policy.Revision < m.Receipt.Policy.Revision || policy.HouseholdID != m.HouseholdID || policy.Source.Key() != m.Receipt.Policy.Source.Key() {
			return storedIntegrity("product", "terms receipt policy does not belong to current facts")
		}
		// Later revisions cannot be substituted for an older immutable result.
		// Compare live facts only where that exact revision still exists.
		expectedContract, expectedPolicy := r, m.Receipt.Policy
		if c.Revision == r.Revision {
			expectedContract = c
		}
		if policy.Revision == m.Receipt.Policy.Revision {
			expectedPolicy = policy
		}
		if same, err := sameProductTermsFacts(m.Receipt, expectedContract, expectedPolicy); err != nil {
			return err
		} else if !same {
			return storedIntegrity("product", "terms receipt disagrees with the same recorded revision")
		}
	}
	return nil
}
