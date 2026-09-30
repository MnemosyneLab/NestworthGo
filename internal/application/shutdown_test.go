package application

import (
	"context"
	"testing"
	"time"
)

func TestShutdownFencesNewWritesAndWaitsForExistingWriter(t *testing.T) {
	service := NewService(nil)
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- service.WithWrite(context.Background(), func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	shutdownDone := make(chan struct{})
	go func() { service.QuiesceWritesForShutdown(); close(shutdownDone) }()
	rejected := make(chan error, 1)
	go func() {
		rejected <- service.WithWrite(context.Background(), func(context.Context) error { t.Error("new write entered during shutdown"); return nil })
	}()
	select {
	case err := <-rejected:
		if err == nil {
			t.Fatal("new write accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("new writer was not rejected before old writer finished")
	}
	select {
	case <-shutdownDone:
		t.Fatal("shutdown did not wait for active writer")
	default:
	}
	close(release)
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not drain writer")
	}
	// Idempotence does not reopen the gate.
	service.QuiesceWritesForShutdown()
	if err := service.WithWrite(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("shutdown reopened writes")
	}
}
func TestShutdownDoesNotDeadlockAfterRestoreKeptExclusiveMutex(t *testing.T) {
	service := NewService(nil)
	if err := service.WithExclusiveKeep(context.Background(), ExclusiveRestore, func(context.Context) (bool, error) { service.LockWrites(); return true, nil }); err != nil {
		t.Fatal(err)
	}
	defer service.UnlockWrites()
	done := make(chan struct{})
	go func() { service.QuiesceWritesForShutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown tried to acquire retained restore mutex/permit")
	}
	if err := service.WithWrite(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("restore shutdown admitted write")
	}
}
