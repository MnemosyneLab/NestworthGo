package desktop

import (
	"context"
	"errors"
	"image"
	"slices"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/banklogos"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestInstitutionIconChoicesMatchBankLogoAvailability(t *testing.T) {
	presentations, _, err := banklogos.Load()
	if err != nil {
		t.Fatal(err)
	}
	keys := make(map[string]bool, len(institutionIconChoices[i18n.English]))
	for _, choice := range institutionIconChoices[i18n.English] {
		keys[choice.ID] = true
		if choice.Glyph == nil {
			t.Errorf("%s has no glyph", choice.ID)
		}
	}
	for _, presentation := range presentations {
		if keys[presentation.Key] != presentation.Selectable() {
			t.Errorf("%s: expected selectable = %v", presentation.Key, presentation.Selectable())
		}
		for _, alias := range presentation.LegacyKeys {
			if got := directoryBankLogos[alias]; got.ID != presentation.Key || got.Glyph == nil {
				t.Errorf("legacy key %q did not resolve to %q", alias, presentation.Key)
			}
		}
	}
}

type institutionIconService struct {
	*application.Service
	edits, creates int
	failReload     bool
}

func (s *institutionIconService) CreateInstitution(ctx context.Context, name string, kind domain.InstitutionType, icons ...string) (domain.Institution, error) {
	s.creates++
	return s.Service.CreateInstitution(ctx, name, kind, icons...)
}

func (s *institutionIconService) EditInstitution(ctx context.Context, id domain.InstitutionID, edit application.DirectoryEdit) (domain.Institution, error) {
	s.edits++
	return s.Service.EditInstitution(ctx, id, edit)
}

func (s *institutionIconService) ListInstitutions(ctx context.Context, includeArchived bool) ([]domain.Institution, error) {
	if s.failReload {
		s.failReload = false
		return nil, errors.New("injected institution reload failure")
	}
	return s.Service.ListInstitutions(ctx, includeArchived)
}

func TestInstitutionIconsFollowTypeThenPersistExplicitChoice(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	service, close := open()
	check := func(name string, kind domain.InstitutionType, icon string) domain.Institution {
		t.Helper()
		rows, err := service.ListInstitutions(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(rows, func(row domain.Institution) bool { return row.Name == name })
		if i < 0 || rows[i].InstitutionType != kind || rows[i].IconKey == nil || *rows[i].IconKey != icon {
			t.Fatalf("%q did not have type %q and icon %q", name, kind, icon)
		}
		return rows[i]
	}
	svc := &institutionIconService{Service: service}
	p := newInstitutionsPage(ctx, svc, func() {})
	directory := newDirectoryPage(p)
	t.Cleanup(directory.wait)
	h := newShellHarnessWithPages(t, ledgerPages{directory: directory})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Directory", false)
	h.size = image.Pt(700, 460)
	finish := func() {
		h.frame()
		directory.wait()
		h.frame()
	}
	finish()

	searchDirectory(h, "Search institutions", "Icon Test")
	h.click("Add institution", false)
	h.router.Queue(key.EditEvent{Text: "Icon Test Institution"})
	h.frame()
	h.click("Institution type", true)
	h.click("Insurer", false)
	h.frame()
	if p.icon != domain.DefaultIconForInstitutionType(domain.InstitutionInsurer) {
		t.Fatalf("type selection did not update the default icon: %q", p.icon)
	}

	const bankKey = "bank-logo:standard-chartered"
	chooseDirectoryIcon(h, "ＳＴＡＮＤＡＲＤ_Chartered", directoryBankLogos[bankKey].Label)
	h.click("Institution type", true)
	h.click("Brokerage", false)
	h.frame()
	if p.icon != bankKey {
		t.Fatalf("type change overwrote an explicitly selected symbol: %q", p.icon)
	}
	h.click("Add", false)
	finish()
	created := check("Icon Test Institution", domain.InstitutionBrokerage, bankKey)
	assertPageLabel(t, &h.router, created.Name)
	h.click("View", false)
	finish()
	if directory.object.row == nil || directory.object.row.iconKey() != bankKey ||
		p.glyph(directory.object.row.iconKey(), directory.object.row.institutionType) != directoryBankLogos[bankKey].Glyph {
		t.Fatal("object visit did not resolve the saved symbol")
	}
	h.click("Back", false)
	finish()
	h.click("Edit", false)
	h.frame()
	chooseDirectoryIcon(h, "Home", "Home")
	h.click("Institution name", true)
	replaceDirectoryName(h, p, "")
	h.click("Save", false)
	finish()
	assertPageLabel(t, &h.router, i18n.English.Text(i18n.PreviewNameRequired))
	check(created.Name, domain.InstitutionBrokerage, bankKey)

	replaceDirectoryName(h, p, "Icon Test Renamed")
	svc.failReload = true
	h.click("Save", false)
	finish()
	assertPageLabel(t, &h.router, "The change was saved. Retry to reload the list.")
	h.click("Retry reload", false)
	finish()
	assertPageLabel(t, &h.router, "Icon Test Renamed")
	if svc.edits != 2 || svc.creates != 1 {
		t.Fatalf("unexpected write counts: %d creations, %d edits", svc.creates, svc.edits)
	}

	close()
	service, close = open()
	defer close()
	if reopened := check("Icon Test Renamed", domain.InstitutionBrokerage, "home"); reopened.ID != created.ID {
		t.Fatal("rename or icon update replaced the institution ID")
	}
}

func TestInstitutionRenameRetainsLegacyIconKey(t *testing.T) {
	ctx, open := newDirectoryFixture(t)
	service, close := open()
	defer close()
	const legacy = "bank-logo:abchina"
	row, err := service.CreateInstitution(ctx, "Legacy Icon", domain.InstitutionBank, legacy)
	if err != nil {
		t.Fatal(err)
	}
	p := newInstitutionsPage(ctx, service, func() {})
	page := newDirectoryPage(p)
	h := newShellHarnessWithPages(t, ledgerPages{directory: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Directory", false)
	page.wait()
	h.frame()
	searchDirectory(h, "Search institutions", "Legacy Icon")
	h.click("Edit", false)
	h.frame()
	label := directoryBankLogos[legacy].Label
	assertPageLabel(t, &h.router, label)
	if p.glyph(legacy, domain.InstitutionBank) != p.glyph(directoryBankLogos[legacy].ID, domain.InstitutionBank) {
		t.Fatal("legacy symbol did not resolve to its canonical image")
	}
	replaceDirectoryName(h, p, "Legacy Icon Renamed")
	h.click("Save", false)
	page.wait()
	h.frame()
	rows, err := service.ListInstitutions(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(rows, func(item domain.Institution) bool { return item.ID == row.ID })
	if i < 0 || rows[i].Name != "Legacy Icon Renamed" || rows[i].IconKey == nil || *rows[i].IconKey != legacy {
		t.Fatalf("name-only save rewrote the legacy key: %v", rows)
	}
}
