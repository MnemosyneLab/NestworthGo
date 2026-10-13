package sqlite

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestInstrumentMetadataTransactionFaultRollsBackEveryRow(t *testing.T) {
	database, repository, household, _, first := seedPortfolioRepository(t)
	second := first
	second.ID = domain.NewInstrumentID()
	second.Name = "Second"
	second.ProviderKey, second.ProviderSymbol = nil, nil
	if err := repository.CreateInstrument(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	before, err := repository.ReadInstrumentMetadata(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(t.Context(), `CREATE TRIGGER fail_second_metadata BEFORE UPDATE OF name ON instruments WHEN NEW.id='`+second.ID.String()+`' BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	items := []domain.InstrumentMetadataUpdate{{ID: first.ID.String(), ExpectedMetadataVersion: domain.InstrumentMetadataVersion(first, false), Changes: domain.InstrumentMetadataChanges{Name: json.RawMessage(`"Updated first"`)}}, {ID: second.ID.String(), ExpectedMetadataVersion: domain.InstrumentMetadataVersion(second, false), Changes: domain.InstrumentMetadataChanges{Name: json.RawMessage(`"Updated second"`)}}}
	if _, err := repository.UpdateInstrumentMetadataBatch(t.Context(), household.ID, items, time.Now()); err == nil {
		t.Fatal("transaction fault ignored")
	}
	after, err := repository.ReadInstrumentMetadata(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("partial instrument transaction")
	}
}
func TestReadSQLBudgetBeforeExpansion(t *testing.T) {
	database, _, _, _, _ := seedPortfolioRepository(t)
	if _, err := database.SQL.ExecContext(t.Context(), `CREATE TABLE read_budget_test(id INTEGER); WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<20001) INSERT INTO read_budget_test SELECT x FROM n`); err != nil {
		t.Fatal(err)
	}
	ctx := domain.WithBoundedRead(t.Context())
	err := checkReadTableBudget(ctx, database.SQL, "read_budget_test", "1=1")
	if err == nil {
		t.Fatal("SQL read budget ignored")
	}
	if code, ok := err.(*domain.Error); !ok || code.Code != "too_large" {
		t.Fatal(err)
	}
}
