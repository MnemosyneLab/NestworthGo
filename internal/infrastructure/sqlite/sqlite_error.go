package sqlite

import (
	"errors"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	moderncsqlite "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"
)

// mapSQLiteError converts driver constraint failures into domain errors at the
// persistence boundary so Wails can surface a stable code instead of a generic
// internal error. SQL text, file paths, and driver codes never leave this
// helper. Non-constraint failures and errors that are already *domain.Error
// are returned unchanged.
func mapSQLiteError(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		return err
	}
	var sqliteErr *moderncsqlite.Error
	if !errors.As(err, &sqliteErr) {
		return err
	}
	code := sqliteErr.Code()
	if code&0xff != sqlite3lib.SQLITE_CONSTRAINT {
		return err
	}
	switch {
	case code == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3lib.SQLITE_CONSTRAINT_PRIMARYKEY:
		return &domain.Error{Code: domain.ErrConflict, Message: "this value is already in use"}
	case code == sqlite3lib.SQLITE_CONSTRAINT_FOREIGNKEY:
		return &domain.Error{Code: domain.ErrValidation, Message: "a referenced record is missing"}
	case code == sqlite3lib.SQLITE_CONSTRAINT_NOTNULL:
		return &domain.Error{Code: domain.ErrValidation, Message: "a required value is missing"}
	case isCheckConstraint(code, sqliteErr.Error()):
		if strings.Contains(strings.ToLower(sqliteErr.Error()), "accounts") {
			return &domain.Error{Code: domain.ErrValidation, Field: "accountType", Message: "is not a valid account combination"}
		}
		return &domain.Error{Code: domain.ErrValidation, Message: "this value does not satisfy a stored constraint"}
	default:
		return &domain.Error{Code: domain.ErrValidation, Message: "this value does not satisfy a stored constraint"}
	}
}

func storedIntegrity(field, message string) *domain.Error {
	return &domain.Error{Code: domain.ErrIntegrity, Field: field, Message: message}
}

func asStoredIntegrity(field string, err error) *domain.Error {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		return storedIntegrity(field, domainErr.Message)
	}
	return storedIntegrity(field, err.Error())
}

func isCheckConstraint(code int, message string) bool {
	if code == sqlite3lib.SQLITE_CONSTRAINT_CHECK {
		return true
	}
	return strings.Contains(strings.ToLower(message), "check constraint")
}
