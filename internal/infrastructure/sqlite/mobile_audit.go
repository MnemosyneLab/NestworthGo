package sqlite

import (
	"context"
	"database/sql"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func saveMobileAuditTx(ctx context.Context, tx *sql.Tx, hh domain.HouseholdID, mutation domain.MutationID, a domain.MobileAudit) error {
	raw, err := jsoncodec.Marshal(a)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_configuration(key,value) VALUES(?,?)`, "mobile.record-audit.v1."+hh.String()+"."+mutation.String(), string(raw))
	return err
}
