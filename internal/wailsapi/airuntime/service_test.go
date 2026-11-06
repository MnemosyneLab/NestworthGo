package airuntime

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"github.com/waltwang/nestworth-go/internal/infrastructure/aicredentials"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInstalledIPCExposesDesktopConfigurationWithoutFinancialWrites(t *testing.T) {
	typ := reflect.TypeFor[*Service]()
	expected := map[string]bool{"Usage": true, "Models": true, "Transcribe": true, "PendingPageOperations": true, "ClaimPageOperation": true, "DeliverPageOperation": true, "SetFullAccess": true, "Reviews": true, "Rename": true, "PendingPageReads": true, "DeliverPageContent": true, "PublishPage": true, "ConsumeNavigation": true, "Configure": true, "SaveCredential": true, "DeleteCredential": true, "Status": true, "List": true, "Create": true, "Inspect": true, "Messages": true, "Start": true, "Cancel": true, "Events": true, "Delete": true}
	if typ.NumMethod() != len(expected) {
		t.Fatal("review IPC inventory")
	}
	for i := 0; i < typ.NumMethod(); i++ {
		if !expected[typ.Method(i).Name] {
			t.Fatal("unsafe IPC method")
		}
	}
	s := NewService(nil)
	if _, err := s.Usage("", assistant.UsageQuery{}); err == nil {
		t.Fatal("unavailable runtime exposed usage")
	}
	if _, err := s.Models("", 0); err == nil {
		t.Fatal("unavailable runtime listed models")
	}
	if s.Status().Available || s.Status().FullAccess {
		t.Fatal("closed service advertised authority")
	}
	if _, e := s.Start(assistant.StartRequest{}); e == nil {
		t.Fatal("unavailable runtime started")
	}
}

type memoryState struct{ state assistant.State }

func (m *memoryState) Load() (assistant.State, error) { return m.state, nil }
func (m *memoryState) Save(s assistant.State) error   { m.state = s; return nil }
func (*memoryState) Close() error                     { return nil }

func TestDatabaseCredentialsThroughDesktopIPCKeepActivationClosed(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewConfigurationRepository(db)
	// Existing configuration belongs to other services and must survive untouched.
	if err := repo.SaveConfiguration("settings", []byte("synthetic-existing-provider")); err != nil {
		t.Fatal(err)
	}
	state := &memoryState{state: assistant.InitialState("synthetic-ledger")}
	runtime, err := assistant.NewRuntime(assistant.Options{Ledger: "synthetic-ledger", Store: state, Credentials: aicredentials.New(repo), Validate: func(context.Context) bool { return true }, Factory: func(context.Context, assistant.ModelConfig, []byte, string) (assistant.Engine, error) {
		t.Error("credential settings called provider")
		return nil, assistant.ErrUnavailable
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	service := NewService(runtime)
	epoch := service.Status().Epoch
	if service.Status().Credentials != "missing" {
		t.Fatal("unexpected initial status")
	}
	if service.SaveCredential("stale", "synthetic-first-key") == nil {
		t.Fatal("stale host accepted")
	}
	for _, key := range []string{"synthetic-first-key", "synthetic-replaced-key"} {
		if err := service.SaveCredential(epoch, key); err != nil {
			t.Fatal(err)
		}
		status := service.Status()
		raw, err := jsoncodec.Marshal([]any{status, state.state})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), key) || status.Credentials != "present" || status.Available || status.FullAccess || status.Config.Enabled {
			t.Fatal("credential leaked or activation opened")
		}
	}
	conversation, err := service.Create(epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(assistant.StartRequest{Epoch: epoch, Revision: service.Status().Revision, ConversationID: conversation.ID, RequestID: uuid.NewString(), Prompt: "synthetic financial question"}); err == nil {
		t.Fatal("saving key activated provider")
	}
	if err := service.DeleteCredential(epoch); err != nil {
		t.Fatal(err)
	}
	if service.Status().Credentials != "missing" || service.Status().Config.Enabled {
		t.Fatal("delete did not revoke")
	}
	existing, found, err := repo.LoadConfiguration("settings")
	if err != nil || !found || string(existing) != "synthetic-existing-provider" {
		t.Fatal("modified existing provider settings")
	}
}

// This fixture is consumed by the frontend too. Verify it against the actual
// nil-runtime DTO so the UI cannot rely on a friendlier hand-written mock.
func TestNilRuntimeStatusWireContract(t *testing.T) {
	service := NewService(nil)
	status := service.Status()
	actual, err := jsoncodec.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../../testdata/assistant/nil-runtime-status.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, bytes.TrimSpace(fixture)) {
		t.Fatalf("nil-runtime wire fixture differs: %s", actual)
	}
	if status.Credentials != "unavailable" || status.Epoch != "" || status.Available || status.FullAccess || status.Config.Enabled {
		t.Fatalf("nil runtime must report closed configuration: %s", actual)
	}
	if !errors.Is(service.SaveCredential(status.Epoch, "synthetic-key"), assistant.ErrUnavailable) || !errors.Is(service.DeleteCredential(status.Epoch), assistant.ErrUnavailable) {
		t.Fatal("nil runtime accepted credential mutation")
	}
}
