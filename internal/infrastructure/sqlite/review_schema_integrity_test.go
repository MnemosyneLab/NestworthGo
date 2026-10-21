package sqlite

import (
	"path/filepath"
	"testing"
)

func TestCurrentReviewSchemaUniquenessTampering(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "synthetic-current.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.SQL.Exec(`DROP INDEX ux_investment_review_open_episode`); err != nil {
		t.Fatal(err)
	}
	if err := db.Verify(t.Context()); err == nil {
		t.Fatal("accepted missing review uniqueness index")
	}
}
