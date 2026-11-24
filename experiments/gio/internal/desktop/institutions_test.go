package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type testInstitutionService struct {
	read  func() ([]domain.Institution, error)
	write func(domain.InstitutionID, string) (domain.Institution, error)
}

func (s testInstitutionService) ListInstitutions(_ context.Context, _ bool) ([]domain.Institution, error) {
	return s.read()
}

func (s testInstitutionService) UpdateInstitution(_ context.Context, id domain.InstitutionID, name string) (domain.Institution, error) {
	return s.write(id, name)
}

func (s testInstitutionService) CreateInstitution(context.Context, string, domain.InstitutionType, ...string) (domain.Institution, error) {
	panic("unexpected institution creation")
}

func (s testInstitutionService) ArchiveInstitution(context.Context, domain.InstitutionID, bool) error {
	panic("unexpected archive change")
}

func TestInstitutionsRouteLoadsFiltersAndRefreshesAfterSave(t *testing.T) {
	bank := domain.Institution{ID: domain.NewInstitutionID(), Name: "Harbor Sample Bank", InstitutionType: domain.InstitutionBank}
	readCalls, writeCalls := 0, 0
	svc := testInstitutionService{
		read: func() ([]domain.Institution, error) {
			readCalls++
			switch readCalls {
			case 1, 4:
				return nil, errors.New("read failure")
			case 2:
				return nil, nil
			case 3:
				return []domain.Institution{bank}, nil
			default:
				row := bank
				row.Name = "Authoritative name"
				return []domain.Institution{row}, nil
			}
		},
		write: func(id domain.InstitutionID, draft string) (domain.Institution, error) {
			writeCalls++
			if id != bank.ID || draft == bank.Name || draft == "" {
				t.Errorf("unexpected save request for %s with draft %q", id, draft)
			}
			if writeCalls == 1 {
				return domain.Institution{}, &domain.Error{Code: domain.ErrValidation, Field: "name", MaxLength: 120}
			}
			result := bank
			result.Name = "Mutation response"
			return result, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := newInstitutionsPage(ctx, svc, func() {})
	t.Cleanup(func() { cancel(); p.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{directory: newDirectoryPage(p)})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	finish := func() {
		p.workers.Wait()
		h.frame()
	}
	h.click("Directory", false)
	if !p.busy || p.operation != directoryRead {
		t.Fatal("the route did not start in the loading state")
	}
	finish()
	assertPageLabel(t, &h.router, "Could not load institutions.")
	h.click("Retry reload", false)
	finish()
	assertPageLabel(t, &h.router, "This household has no active institutions.")
	h.click("Reload", false)
	finish()

	h.click("Search institutions", true)
	h.router.Queue(key.EditEvent{Text: "no match"})
	h.frame()
	assertPageLabel(t, &h.router, "No matching institutions")
	h.click("Clear search", true)
	h.click("Edit", false)
	h.frame()
	h.focused(&p.draft.Editor)
	h.router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: p.draft.Editor.Len()}, Text: "Rejected draft"})
	h.frame()
	h.click("Save", false)
	finish()
	if p.editMode != directoryEdit || p.editID != bank.ID.String() || p.draft.Text() != "Rejected draft" {
		t.Fatal("a validation failure dropped the active edit")
	}
	assertPageLabel(t, &h.router, "Use 120 characters or fewer.")
	selection := key.Range{Start: 8, End: 2}
	h.router.Queue(key.SelectionEvent(selection), key.SnippetEvent{End: p.draft.Editor.Len()})
	h.frame()
	h.selectSidebarLanguage(i18n.SettingsLanguageZhTW)
	h.cycleAppearance()
	start, end := p.draft.Editor.Selection()
	if h.shell.activeRoute() != directoryRoute || p.editMode != directoryEdit || p.editID != bank.ID.String() || p.draft.Text() != "Rejected draft" ||
		start != selection.Start || end != selection.End {
		t.Fatal("switching preferences changed the institution route, draft, or selection")
	}
	semanticBounds(t, &h.router, "機構名稱", true)
	assertPageLabel(t, &h.router, "名稱不得超過 120 個字元。")
	h.click("機構名稱", true)
	h.router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: p.draft.Editor.Len()}, Text: "Valid draft"})
	h.frame()
	h.click("儲存", false)
	finish()
	if p.editMode != directoryRead || p.awaitingRead.id != bank.ID.String() || p.notice != i18n.NoMessage {
		t.Fatal("a committed save with a failed reload did not enter the reload-only state")
	}
	assertPageLabel(t, &h.router, "變更已儲存。請重試載入清單。")
	h.click("重試載入", false)
	finish()
	if writeCalls != 2 || readCalls != 5 || p.notice != i18n.InstitutionsSaved || len(p.rows) != 1 || p.rows[0].Name != "Authoritative name" {
		t.Fatalf("retry did not load the authoritative list: %d writes, %d reads, notice %v, rows %v", writeCalls, readCalls, p.notice, p.rows)
	}
	assertPageLabel(t, &h.router, "Authoritative name")
	h.focused(&p.buttons[bank.ID.String()].edit)
}

func assertPageLabel(t *testing.T, router *input.Router, label string) image.Rectangle {
	t.Helper()
	node := findPageLabel(router, label)
	if node == nil {
		t.Fatalf("expected label %q in the rendered page", label)
	}
	return node.Desc.Bounds
}

func findPageLabel(router *input.Router, label string) *input.SemanticNode {
	var visit func([]input.SemanticNode) *input.SemanticNode
	visit = func(nodes []input.SemanticNode) *input.SemanticNode {
		for i := range nodes {
			node := &nodes[i]
			if node.Desc.Label == label {
				return node
			}
			if child := visit(node.Children); child != nil {
				return child
			}
		}
		return nil
	}
	return visit(router.AppendSemantics(nil))
}

// Faults are injected at the UI/service boundary. Every successful command and
// read still uses the application service and the verified SQLite fixture.
type institutionLifecycleService struct {
	*application.Service
	listFilters       []bool
	archiveCalls      []bool
	failArchiveReload bool
	failNextList      bool
	failRestore       bool
}

func (s *institutionLifecycleService) ListInstitutions(ctx context.Context, includeArchived bool) ([]domain.Institution, error) {
	s.listFilters = append(s.listFilters, includeArchived)
	if s.failNextList {
		s.failNextList = false
		return nil, errors.New("injected list failure")
	}
	return s.Service.ListInstitutions(ctx, includeArchived)
}

func (s *institutionLifecycleService) ArchiveInstitution(ctx context.Context, id domain.InstitutionID, archived bool) error {
	s.archiveCalls = append(s.archiveCalls, archived)
	if !archived && s.failRestore {
		s.failRestore = false
		return errors.New("injected restore failure")
	}
	err := s.Service.ArchiveInstitution(ctx, id, archived)
	if err == nil && archived && s.failArchiveReload {
		s.failNextList = true
	}
	return err
}

func newDirectoryFixture(t *testing.T) (context.Context, func() (*application.Service, func())) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NESTWORTH_GIO_RUN_DIR", root)
	t.Setenv("NESTWORTH_DATABASE_PATH", "")
	t.Setenv("NESTWORTH_SETTINGS_PATH", "")
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	open := func() (*application.Service, func()) {
		t.Helper()
		db, unlock, err := pilot.OpenSession(ctx, paths)
		if err != nil {
			t.Fatal(err)
		}
		close := func() {
			t.Helper()
			if db != nil {
				err := errors.Join(db.CheckpointWAL(ctx), db.Close(), unlock())
				db = nil
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		t.Cleanup(close)
		return application.NewService(sqlite.NewRepository(db)), close
	}
	return ctx, open
}

func TestInstitutionsLifecycleKeepsSearchAndPersistsAcrossReopen(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	openDirectory := func(service institutionService) (*previewHarness, *directoryList, func()) {
		t.Helper()
		p := newInstitutionsPage(ctx, service, func() {})
		t.Cleanup(p.workers.Wait)
		h := newShellHarnessWithPages(t, ledgerPages{directory: newDirectoryPage(p)})
		h.shell.prefs.saved.Language = settings.LanguageEnglish
		h.frame()
		finish := func() {
			p.workers.Wait()
			h.frame()
		}
		h.click("Directory", false)
		if h.shell.activeRoute() != directoryRoute {
			t.Fatalf("could not open Directory from the sidebar at %s: active route %s", h.size, h.shell.activeRoute())
		}
		h.size = image.Pt(700, 460)
		finish()
		return h, p, finish
	}

	const name = "Lifecycle Sample"
	checkRow := func(service *application.Service, id domain.InstitutionID, archived bool) domain.Institution {
		t.Helper()
		rows, err := service.ListInstitutions(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		found := slices.IndexFunc(rows, func(row domain.Institution) bool { return row.Name == name })
		if found == -1 {
			t.Fatalf("the institution was missing: %v", rows)
		}
		row := rows[found]
		if id != "" && row.ID != id || (row.ArchivedAt != nil) != archived ||
			row.InstitutionType != domain.InstitutionInsurer || row.IconKey == nil ||
			*row.IconKey != domain.DefaultIconForInstitutionType(domain.InstitutionInsurer) {
			t.Fatalf("unexpected stored institution: %+v", row)
		}
		return row
	}
	search := func(h *previewHarness, expected string) {
		t.Helper()
		h.click("Search institutions", true)
		h.router.Queue(key.EditEvent{Text: "  LIFECYCLE  "})
		h.frame()
		assertPageLabel(t, &h.router, expected)
	}

	service, close := open()
	s := &institutionLifecycleService{Service: service, failArchiveReload: true}
	h, p, finish := openDirectory(s)
	assertPageLabel(t, &h.router, "Harbor Sample Bank")
	search(h, "No matching institutions")
	h.click("Add institution", false)
	h.focused(&p.draft.Editor)
	h.click("Add", false)
	finish()
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.PreviewNameRequired))
	h.click("Institution type", true)
	h.press(key.NameDownArrow)
	h.press(key.NameDownArrow)
	h.press(key.NameReturn)
	h.click("Institution name", true)
	h.router.Queue(key.EditEvent{Text: "  " + name + "  "})
	h.frame()
	h.click("Add", false)
	finish()
	assertPageLabel(t, &h.router, name)
	assertPageLabel(t, &h.router, "Institution added.")
	row := checkRow(service, "", false)
	h.focused(&p.buttons[row.ID.String()].edit)

	h.click("Archive", false)
	assertPageLabel(t, &h.router, "Archive institution?")
	h.focused(&p.confirmation.Cancel)
	h.press(key.NameTab)
	h.focused(&p.confirmation.Confirm)
	h.press(key.NameEscape)
	h.focused(&p.buttons[row.ID.String()].archive)
	if len(s.archiveCalls) != 0 {
		t.Fatal("dismissing the confirmation submitted the archive")
	}
	h.press(key.NameReturn)
	h.press(key.NameTab)
	h.press(key.NameReturn)
	finish()
	assertPageLabel(t, &h.router, "The change was saved. Retry to reload the list.")
	assertPageLabel(t, &h.router, name)
	// The row stays readable, while its stale action cannot resubmit the committed command.
	archiveLabel := assertPageLabel(t, &h.router, "Archive")
	h.clickAt(archiveLabel.Min.Add(archiveLabel.Max).Div(2))
	h.click("Retry reload", false)
	finish()
	assertPageLabel(t, &h.router, "Institution archived.")
	assertPageLabel(t, &h.router, "No matching institutions")
	if findPageLabel(&h.router, name) != nil || !slices.Equal(s.archiveCalls, []bool{true}) {
		t.Fatalf("active results or writes are incorrect after retry: %v", s.archiveCalls)
	}
	h.focused(&p.search.Field.Editor)
	h.click("Show archived", false)
	finish()
	assertPageLabel(t, &h.router, name)
	assertPageLabel(t, &h.router, "Insurer · Archived")
	if findPageLabel(&h.router, "Harbor Sample Bank") != nil || !slices.Equal(s.listFilters, []bool{false, false, false, false, true}) {
		t.Fatalf("showing archived did not preserve the search or use the right queries: %v", s.listFilters)
	}
	h.size = image.Pt(1120, 840)
	h.frame()
	h.click("Clear search", true)
	assertPageLabel(t, &h.router, "Harbor Sample Bank")
	assertPageLabel(t, &h.router, "Insurer · Archived")
	checkRow(service, row.ID, true)
	close()

	service, close = open()
	checkRow(service, row.ID, true)
	s = &institutionLifecycleService{Service: service, failRestore: true}
	h, p, finish = openDirectory(s)
	search(h, "No matching institutions")
	h.click("Show archived", false)
	finish()
	assertPageLabel(t, &h.router, "Insurer · Archived")
	h.click("Restore", false)
	finish()
	assertPageLabel(t, &h.router, "Could not restore the institution. Try again.")
	assertPageLabel(t, &h.router, "Insurer · Archived")
	h.focused(&p.buttons[row.ID.String()].archive)
	h.press(key.NameReturn)
	finish()
	assertPageLabel(t, &h.router, "Institution restored.")
	assertPageLabel(t, &h.router, name)
	semanticBounds(t, &h.router, "Archive", false)
	if findPageLabel(&h.router, "Insurer · Archived") != nil || !slices.Equal(s.archiveCalls, []bool{false, false}) {
		t.Fatalf("restore did not update the row or attempted an unexpected command: %v", s.archiveCalls)
	}
	h.click("Hide archived", false)
	finish()
	assertPageLabel(t, &h.router, name)
	checkRow(service, row.ID, false)
	close()

	service, close = open()
	defer close()
	h, _, _ = openDirectory(service)
	search(h, name)
	checkRow(service, row.ID, false)
}
