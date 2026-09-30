package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestDrainForReplicationQueuesReadersAndPreservesPool(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pool := db.SQL
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.DrainForReplication(context.Background(), func() error { close(entered); <-release; return nil })
	}()
	<-entered
	readDone := make(chan error, 1)
	go func() {
		var version int
		err := db.SQL.QueryRow("PRAGMA user_version").Scan(&version)
		if err == nil && version != 15 {
			err = errors.New("wrong schema")
		}
		readDone <- err
	}()
	select {
	case err := <-readDone:
		t.Fatalf("reader entered during close: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if db.SQL != pool {
		t.Fatal("pool pointer changed")
	}
	if _, err := db.SQL.Exec("INSERT INTO app_configuration(key,value) VALUES('drain-probe','{}')"); err != nil {
		t.Fatal(err)
	}
}
func TestDrainTimeoutDoesNotCloseActiveConnection(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.SQL.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	called := false
	err = db.DrainForReplication(ctx, func() error { called = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || called {
		t.Fatalf("unsafe drain: %v %v", err, called)
	}
	if _, err := conn.ExecContext(context.Background(), "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	conn.Close()
}
