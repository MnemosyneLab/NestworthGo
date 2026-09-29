package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestSQLitePlanDoesNotCollideWithOperationReceipt(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "plans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewConfigurationRepository(db)
	s := New(nil, t.TempDir(), nil, repo)
	id := uuid.NewString()
	planPath := filepath.Join(s.dir, "plans", id+".json")
	if err := s.writePrivate(planPath, changePlan{ID: id, Version: "reviewed"}); err != nil {
		t.Fatal(err)
	}
	operationPath, err := s.operationPath(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.writePrivate(operationPath, Operation{ID: id, Tool: "commit_change", Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	// A new service at another legacy path must read the same database plan,
	// while the settings list only includes the actual operation receipt.
	restarted := New(nil, t.TempDir(), nil, repo)
	raw, err := restarted.readPrivate(filepath.Join(restarted.dir, "plans", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan changePlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.ID != id || plan.Version != "reviewed" {
		t.Fatalf("plan was overwritten: %+v", plan)
	}
	operations, err := restarted.RecentOperations()
	if err != nil || len(operations) != 1 || operations[0].Tool != "commit_change" {
		t.Fatalf("operations = %+v, %v", operations, err)
	}
}

func TestSQLiteMigratesConnectionAndReceipts(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "agent")
	db, err := sqlite.Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewConfigurationRepository(db)
	cfg := Config{Mode: ReadOnly, InstanceID: uuid.NewString(), Token: "saved-token", Port: 12345}
	connection := filepath.Join(legacy, "connection.json")
	if err := writePrivateJSON(connection, cfg); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	receipt := filepath.Join(legacy, "operations", id+".json")
	if err := writePrivateJSON(receipt, Operation{ID: id, Status: "pending", Tool: "create_member"}); err != nil {
		t.Fatal(err)
	}
	s := New(nil, legacy, nil, repo)
	if err := s.Resume(); err != nil {
		t.Fatal(err)
	}
	if s.config != cfg {
		t.Fatal("connection changed during migration")
	}
	for _, path := range []string{connection, receipt} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("legacy private JSON retained")
		}
	}
	// All directory JSON can disappear; the same database retains identity and receipts.
	restarted := New(nil, filepath.Join(dir, "new-location"), nil, repo)
	if err := restarted.Resume(); err != nil {
		t.Fatal(err)
	}
	if restarted.config != cfg {
		t.Fatal("connection lost after directory change")
	}
	op, err := restarted.GetOperation(id)
	if err != nil || op.Status != "unknown" {
		t.Fatal("pending receipt lost", err)
	}
	recent, err := restarted.RecentOperations()
	if err != nil || len(recent) != 1 {
		t.Fatal("receipt missing", err)
	}
	cfg.Token = ""
	if err := s.writePrivate(connection, cfg); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateJSON(connection, Config{Mode: ReadOnly, Token: "obsolete-token"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Resume(); err != nil {
		t.Fatal(err)
	}
	if s.config.Token != "" {
		t.Fatal("stale legacy token resurrected")
	}
}
