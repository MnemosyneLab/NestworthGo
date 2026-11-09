package mcpserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type processReviewMetadata struct {
	SessionID string `json:"sessionId"`
	ReviewID  string `json:"reviewId"`
	Digest    string `json:"digest"`
}

type dropSuccessfulReceipt struct {
	settings.ConfigurationRepository
	marker string
}

func (r dropSuccessfulReceipt) SaveConfiguration(key string, raw []byte) error {
	if strings.Contains(key, ".operation.") {
		var operation Operation
		if jsoncodec.Unmarshal(raw, &operation) == nil && operation.Status == "succeeded" {
			// The transaction has returned success. Abruptly terminate before
			// saving the outer success receipt or returning to the host caller.
			if err := os.WriteFile(r.marker, []byte("business committed; response dropped"), 0600); err != nil {
				return err
			}
			os.Exit(0)
		}
	}
	return r.ConfigurationRepository.SaveConfiguration(key, raw)
}

// This helper is invoked only by the parent test with its synthetic marker.
// No production launcher, provider request, real ledger or credential is used.
func TestHostReviewProcessChild(t *testing.T) {
	root := os.Getenv("NESTWORTH_AI_PROCESS_TEST_ROOT")
	if root == "" {
		t.Skip("subprocess helper for TestHostReviewAcrossProcessRestart")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("absolute synthetic test directory required")
	}
	marker, err := os.ReadFile(filepath.Join(root, "SYNTHETIC"))
	if err != nil || string(marker) != "host-review synthetic only" {
		t.Fatal("synthetic marker missing")
	}
	phase := os.Getenv("NESTWORTH_AI_PROCESS_TEST_PHASE")
	app, db := openLocalApp(t, filepath.Join(root, "synthetic.db"))
	metadataPath := filepath.Join(root, "review.json")
	var metadata processReviewMetadata
	if phase == "prepare" || phase == "commit_drop" {
		fx := newLedgerFixtureWithApp(t, app, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
		id := uuid.NewString()
		local, err := NewInProcessSession(t.Context(), app, root, id, ReadWrite, nil, sqlite.NewConfigurationRepository(db))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(local.Close)
		fixture := hostFixture{ledgerFixture: fx, local: local, db: db, dir: root, id: id}
		review := fullPreview(t, fixture)
		metadata = processReviewMetadata{SessionID: id, ReviewID: review.Binding.ID, Digest: review.Digest}
		raw, err := jsoncodec.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(metadataPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if phase == "commit_drop" {
			local.service.repository = dropSuccessfulReceipt{local.service.repository, filepath.Join(root, "DROPPED")}
			_, err := processSubmit(t, local, review.Binding.ID, review.Digest)
			t.Fatalf("expected process exit after business commit, got %v", err)
		}
		// Preserve the pending preview through a real process exit, skipping
		// deferred session/database cleanup as in an interrupted application.
		os.Exit(0)
	}
	if phase != "resume_unconfirmed" && phase != "resume_committed" {
		t.Fatal("unknown child phase")
	}
	raw, err := os.ReadFile(metadataPath)
	if err != nil || jsoncodec.Unmarshal(raw, &metadata) != nil {
		t.Fatal("private test metadata unavailable")
	}
	bootstrap, err := app.Bootstrap(t.Context())
	if err != nil || bootstrap.Household == nil || bootstrap.Household.Name != "MCP ledger" {
		t.Fatal("synthetic household verification failed")
	}
	local, err := NewInProcessSession(t.Context(), app, root, metadata.SessionID, ReadWrite, nil, sqlite.NewConfigurationRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(local.Close)
	version := app.MobileDataVersion()
	review, err := local.InspectHostChange(t.Context(), metadata.ReviewID)
	if err != nil || version != app.MobileDataVersion() {
		t.Fatalf("recovery attempted a financial write: %v", err)
	}
	count := 0
	if phase == "resume_unconfirmed" {
		if review.State != ReviewNeedsPreview {
			t.Fatal("process restart retained uncommitted review authority")
		}
		_, err := processSubmit(t, local, metadata.ReviewID, metadata.Digest)
		requireLocalError(t, err, "review_required")
	} else {
		count = 1
		if review.State != ReviewSucceeded || review.Receipt == nil || !strings.Contains(string(review.Receipt.Result), `"12.34"`) {
			t.Fatal("transaction-backed success not recovered after abrupt exit")
		}
		if _, err := processSubmit(t, local, metadata.ReviewID, metadata.Digest); err != nil || version != app.MobileDataVersion() {
			t.Fatalf("repeated confirmation redispatched after restart: %v", err)
		}
	}
	activities, err := app.ListActivities(t.Context(), 100)
	if err != nil || len(activities) != count {
		t.Fatalf("process recovery duplicated/lost activity: count=%d err=%v", len(activities), err)
	}
}

func processSubmit(t *testing.T, local *InProcessSession, id, _ string) (HostChangeReview, error) {
	return local.ExecuteFullChange(t.Context(), id)
}

func TestHostReviewAcrossProcessRestart(t *testing.T) {
	hostReviewAcrossProcessRestart(t)
}
func hostReviewAcrossProcessRestart(t *testing.T) {
	for _, scenario := range []struct{ prepare, resume string }{{"prepare", "resume_unconfirmed"}, {"commit_drop", "resume_committed"}} {
		t.Run(scenario.resume, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "SYNTHETIC"), []byte("host-review synthetic only"), 0600); err != nil {
				t.Fatal(err)
			}
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			for _, phase := range []string{scenario.prepare, scenario.resume} {
				ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
				command := exec.CommandContext(ctx, binary, "-test.run=^TestHostReviewProcessChild$", "-test.timeout=25s")
				command.Env = append(os.Environ(), "NESTWORTH_AI_PROCESS_TEST_ROOT="+root, "NESTWORTH_AI_PROCESS_TEST_PHASE="+phase)
				output, err := command.CombinedOutput()
				cancel()
				if err != nil {
					t.Fatalf("%s process failed: %v\n%s", phase, err, output)
				}
			}
			if scenario.prepare == "commit_drop" {
				if marker, err := os.ReadFile(filepath.Join(root, "DROPPED")); err != nil || string(marker) != "business committed; response dropped" {
					t.Fatal("fixture did not reach the lost-response boundary")
				}
			}
		})
	}
}
