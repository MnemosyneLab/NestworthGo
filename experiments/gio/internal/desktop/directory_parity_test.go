package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestDirectoryParityAcrossEntityKinds(t *testing.T) {
	for _, tc := range []struct {
		tab, add, name string
		index          int
		kind           domain.InstitutionType
	}{
		{tab: "Members", add: "Add a member", name: "Member name"},
		{tab: "Institutions", add: "Add an institution", name: "Institution name", index: 1, kind: domain.InstitutionBank},
		{tab: "Groups", add: "Add a group", name: "Group name", index: 2},
	} {
		t.Run(tc.tab, func(t *testing.T) {
			ctx, open := newDirectoryFixture(t)
			service, _ := open()
			page := newDirectoryPage(
				newMembersList(ctx, service, func() {}),
				newInstitutionsPage(ctx, service, func() {}),
				newGroupsList(ctx, service, func() {}),
			)
			t.Cleanup(page.wait)
			p := page.lists[tc.index]
			rows := func() []directoryRow {
				t.Helper()
				r, err := p.source.list(ctx, true)
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			icon := p.fallbackIcon(tc.kind)
			type sample struct{ id, name string }
			var seeded []sample
			for i := range 9 {
				name := fmt.Sprintf("Synthetic directory entry %02d", i)
				id, err := p.source.create(ctx, directoryChange{name: name, icon: &icon, kind: tc.kind})
				if err != nil {
					t.Fatal(err)
				}
				seeded = append(seeded, sample{id, name})
			}
			before := rows()
			const maxName = 120
			tooLong := strings.Repeat("界", maxName+1)
			for _, name := range []string{"   ", tooLong} {
				_, err := p.source.create(ctx, directoryChange{name: name, icon: &icon, kind: tc.kind})
				var invalid *domain.Error
				if !errors.As(err, &invalid) || invalid.Code != domain.ErrValidation || invalid.Field != "name" ||
					(name == tooLong && invalid.MaxLength != maxName) {
					t.Fatalf("shared validation returned %v for name of %d characters", err, len([]rune(name)))
				}
			}
			create := p.source.create
			failNext := false
			var failedCreates int
			p.source.create = func(ctx context.Context, change directoryChange) (string, error) {
				if failNext {
					failNext = false
					failedCreates++
					return "", errors.New("synthetic directory create failure")
				}
				return create(ctx, change)
			}
			h := newShellHarnessWithPages(t, ledgerPages{directory: page})
			h.shell.prefs.saved.Language = settings.LanguageEnglish
			settle := func() {
				for range 3 {
					h.frame()
					page.wait()
				}
				h.frame()
			}
			h.frame()
			h.click("Directory", false)
			settle()
			if h.shell.activeRoute() != directoryRoute {
				t.Fatal("the harness did not open Directory")
			}
			if tc.index != 0 {
				h.click(tc.tab, false)
				settle()
			}
			if page.activeList() != p {
				t.Fatalf("%s did not open its directory list", tc.tab)
			}
			h.size = image.Pt(1200, 525)
			h.frame()
			p.list.ScrollTo(len(before) - 4)
			h.frame()
			position := p.list.Position
			if position.First == 0 {
				t.Fatal("the fixture did not scroll the directory list")
			}
			h.click(tc.add, false)
			settle()
			if p.editMode != directoryCreate {
				t.Fatal("Add did not open the create sheet")
			}
			h.focused(&p.draft.Editor)
			if field := semanticBounds(t, &h.router, tc.name, true); field.Min.X <= h.size.X/2 {
				t.Fatalf("the create form was not in the side sheet: %+v", field)
			}
			replaceDirectoryName(h, p, "Unsubmitted synthetic entry")
			h.press(key.NameEscape)
			settle()
			h.focused(&p.add)
			if p.editMode != directoryRead || p.list.Position != position || !reflect.DeepEqual(before, rows()) {
				t.Fatal("Escape changed directory data or the source list position")
			}

			h.click(tc.add, false)
			settle()
			if p.draft.Text() != "Unsubmitted synthetic entry" {
				t.Fatal("reopening the sheet dropped the unsubmitted draft")
			}
			replaceDirectoryName(h, p, "   ")
			h.click("Add", false)
			settle()
			assertPageLabel(t, &h.router, "Enter a name before saving.")
			if p.editMode != directoryCreate || !reflect.DeepEqual(before, rows()) {
				t.Fatal("a blank name escaped the create sheet or changed the directory")
			}
			replaceDirectoryName(h, p, tooLong)
			h.click("Add", false)
			settle()
			assertPageLabel(t, &h.router, "Name: Use 120 characters or fewer.")
			if p.editMode != directoryCreate || !reflect.DeepEqual(before, rows()) {
				t.Fatal("an oversized name escaped the create sheet or changed the directory")
			}
			duplicate := seeded[0]
			replaceDirectoryName(h, p, duplicate.name)
			failNext = true
			h.click("Add", false)
			settle()
			assertPageLabel(t, &h.router, "The entry could not be added.")
			h.focused(&p.draft.Editor)
			if failedCreates != 1 || p.editMode != directoryCreate || p.draft.Text() != duplicate.name || !reflect.DeepEqual(before, rows()) {
				t.Fatal("the create failure wrote data or discarded the draft")
			}
			h.click("Add", false)
			settle()
			h.focused(&p.add)
			after := rows()
			var matches []string
			for _, row := range after {
				if row.Name == duplicate.name {
					matches = append(matches, row.ID)
				}
			}
			if p.editMode != directoryRead || len(after) != len(before)+1 || len(matches) != 2 || matches[0] == matches[1] ||
				!slices.Contains(matches, duplicate.id) {
				t.Fatalf("retry did not create a distinct entry with the same name: %v", matches)
			}

			h.size.Y = 840
			p.list.ScrollTo(0)
			h.frame()
			target := seeded[1]
			row := assertPageLabel(t, &h.router, target.name)
			edit := &p.buttons[target.id].edit
			edit.Click()
			settle()
			if p.editMode != directoryEdit {
				t.Fatal("Edit did not open the selected row")
			}
			field := semanticBounds(t, &h.router, tc.name, true)
			if field.Min.X >= h.size.X/2 || field.Min.Y < row.Min.Y-24 || field.Min.Y > row.Min.Y+36 ||
				findPageLabel(&h.router, target.name) != nil {
				t.Fatalf("the editor did not replace its row: row %+v, editor %+v", row, field)
			}
			replaceDirectoryName(h, p, "Unsubmitted synthetic rename")
			h.press(key.NameEscape)
			settle()
			h.focused(edit)
			if p.editMode != directoryRead || !reflect.DeepEqual(after, rows()) {
				t.Fatal("canceling the inline edit changed the entry")
			}
			edit.Click()
			settle()
			renamed := target.name + " renamed"
			replaceDirectoryName(h, p, renamed)
			h.click("Save", false)
			settle()
			h.focused(edit)
			directoryParityRow(t, rows(), target.id, renamed, false)

			if err := p.source.archive(ctx, target.id, true); err != nil {
				t.Fatal(err)
			}
			h.click("Show archived", false)
			settle()
			assertPageLabel(t, &h.router, "Restoring an item does not restore related accounts or change existing ownership.")
			h.click("Restore", false)
			settle()
			directoryParityRow(t, rows(), target.id, renamed, false)
		})
	}
}

func directoryParityRow(t *testing.T, rows []directoryRow, id, name string, archived bool) {
	t.Helper()
	at := slices.IndexFunc(rows, func(row directoryRow) bool { return row.ID == id })
	if at < 0 || rows[at].Name != name || rows[at].archived != archived {
		t.Fatalf("directory entry %q did not retain its name and archive state", name)
	}
}
