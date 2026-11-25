package desktop

import (
	"slices"

	"gioui.org/layout"
)

type directoryVisit struct {
	tab  int
	list directoryListVisit
}

type directoryListVisit struct {
	includeArchived bool
	query           string
	firstID         string
	position        layout.Position
}

func (p *directoryPage) visit() directoryVisit {
	return directoryVisit{tab: p.active, list: p.activeList().visit()}
}

func (p *directoryPage) restore(visit directoryVisit) {
	p.active = visit.tab
	p.activeList().restoreVisit = &visit.list
}

func (p *directoryList) visit() directoryListVisit {
	if p.restoreVisit != nil {
		return *p.restoreVisit
	}
	visit := directoryListVisit{includeArchived: p.includeArchived, query: p.query, position: p.list.Position}
	visible := filterDirectoryRows(p.rows, p.query)
	if i := visit.position.First; i >= 0 && i < len(visible) {
		visit.firstID = visible[i].ID
	}
	return visit
}

func (p *directoryList) setArchiveScope(includeArchived bool) {
	p.includeArchived = includeArchived
	p.list.Position = layout.Position{}
	p.rows, p.loaded, p.readErr = nil, false, nil
	p.start(directoryChange{})
}

func (p *directoryList) applyVisit() {
	visit := p.restoreVisit
	if visit == nil || p.busy {
		return
	}
	if p.query != visit.query {
		p.query = visit.query
		p.search.Field.Editor.SetText(visit.query)
	}
	if p.includeArchived != visit.includeArchived {
		// Wait for the canonical list for this scope. A completed write is
		// already committed, so changing scope can issue only a read.
		p.setArchiveScope(visit.includeArchived)
		return
	}
	visible := filterDirectoryRows(p.rows, p.query)
	p.list.Position = restoredPosition(visit.position, len(visible))
	if visit.firstID != "" {
		if i := slices.IndexFunc(visible, func(row directoryRow) bool { return row.ID == visit.firstID }); i >= 0 {
			p.list.Position.First = i
		}
	}
	p.restoreVisit = nil
}

func restoredPosition(position layout.Position, count int) layout.Position {
	if position.First >= count {
		return layout.Position{First: max(0, count-1)}
	}
	return position
}
