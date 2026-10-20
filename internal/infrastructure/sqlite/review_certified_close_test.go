package sqlite

import (
	"testing"
	"time"
)

func TestReviewCertifiedCloseDoesNotWaitForIdleLeasedConnection(t *testing.T) {
	db := financialConflictDB(t)
	r := NewRepository(db).WithCertifiedReviewCandidate()
	conn, _, err := r.certifiedConnection(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	// No query/transaction is running, but the pool's only connection is
	// checked out. Optional hook cleanup must preserve DB.Close semantics.
	leased, err := db.SQL.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer leased.Close()
	done := make(chan error, 1)
	go func() { done <- db.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		leased.Close()
		<-done
		t.Fatal("hook cleanup blocked Close behind an idle leased connection")
	}
}
