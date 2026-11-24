package main

import (
	"context"
	"errors"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/domain"
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

func TestInstitutionsRouteLoadsFiltersAndRefreshesAfterSave(t *testing.T) {
	h := newShellHarness(t)
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
	h.shell.routes[directoryRoute].body = p.layout
	h.shell.preferences.language = english
	h.frame()
	finish := func() {
		p.workers.Wait()
		h.frame()
	}
	h.click("Directory", false)
	if p.request != institutionLoading {
		t.Fatal("the route did not start in the loading state")
	}
	finish()
	assertInstitutionLabel(t, &h.router, "Could not load institutions.")
	h.click("Retry reload", false)
	finish()
	assertInstitutionLabel(t, &h.router, "This household has no active institutions.")
	h.click("Reload", false)
	finish()

	h.click("Search institutions", true)
	h.router.Queue(key.EditEvent{Text: "no match"})
	h.frame()
	assertInstitutionLabel(t, &h.router, "No matching institutions")
	h.click("Clear search", true)
	h.click("Edit Harbor Sample Bank", true)
	h.frame()
	h.focused(&p.draft.editor)
	h.router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: p.draft.editor.Len()}, Text: "Rejected draft"})
	h.frame()
	h.click("Save", false)
	finish()
	if p.editing != bank.ID || p.draft.text() != "Rejected draft" {
		t.Fatal("a validation failure dropped the active edit")
	}
	assertInstitutionLabel(t, &h.router, "Use 120 characters or fewer.")
	selection := key.Range{Start: 8, End: 2}
	h.router.Queue(key.SelectionEvent(selection), key.SnippetEvent{End: p.draft.editor.Len()})
	h.frame()
	h.click("繁體中文", false)
	h.click("顏色主題：靛藍", false)
	h.click("深色", false)
	if h.shell.activeRoute != directoryRoute || p.editing != bank.ID || p.draft.text() != "Rejected draft" ||
		h.router.EditorState().Selection.Range != selection {
		t.Fatal("switching preferences changed the institution route, draft, or selection")
	}
	semanticBounds(t, &h.router, "機構名稱", true)
	assertInstitutionLabel(t, &h.router, "名稱不得超過 120 個字元。")
	h.router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: p.draft.editor.Len()}, Text: "Valid draft"})
	h.frame()
	h.click("儲存", false)
	finish()
	if p.editing != "" || p.awaitingRead != bank.ID || p.saved {
		t.Fatal("a committed save with a failed reload did not enter the reload-only state")
	}
	assertInstitutionLabel(t, &h.router, "名稱已儲存。請重試載入清單。")
	h.click("重試載入", false)
	finish()
	if writeCalls != 2 || readCalls != 5 || !p.saved || len(p.rows) != 1 || p.rows[0].Name != "Authoritative name" {
		t.Fatalf("retry did not load the authoritative list: %d writes, %d reads, saved %v, rows %v", writeCalls, readCalls, p.saved, p.rows)
	}
	semanticBounds(t, &h.router, "編輯 Authoritative name", true)
	h.focused(p.buttons[bank.ID])
}

func assertInstitutionLabel(t *testing.T, router *input.Router, label string) {
	t.Helper()
	var visit func([]input.SemanticNode) bool
	visit = func(nodes []input.SemanticNode) bool {
		for _, node := range nodes {
			if node.Desc.Label == label || visit(node.Children) {
				return true
			}
		}
		return false
	}
	if !visit(router.AppendSemantics(nil)) {
		t.Fatalf("expected label %q in the rendered page", label)
	}
}
