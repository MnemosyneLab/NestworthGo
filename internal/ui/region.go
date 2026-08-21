package ui

import "fyne.io/fyne/v2"

// region memoizes one widget subtree until its owner explicitly invalidates
// it. Keeping the same CanvasObject preserves widget state such as focus,
// cursor position, and unsaved input across an unrelated content refresh.
type region struct {
	build  func() fyne.CanvasObject
	cached fyne.CanvasObject
}

func newRegion(build func() fyne.CanvasObject) *region {
	return &region{build: build}
}

// Invalidate discards the cached subtree. The next Object call rebuilds it.
func (r *region) Invalidate() {
	r.cached = nil
}

func (r *region) Object() fyne.CanvasObject {
	if r.cached == nil {
		r.cached = r.build()
	}
	return r.cached
}

func (c *Controller) pageRegion(page Page, build func() fyne.CanvasObject) *region {
	if c.regions == nil {
		c.regions = make(map[Page]*region)
	}
	if cached, ok := c.regions[page]; ok {
		return cached
	}
	created := newRegion(build)
	c.regions[page] = created
	return created
}

func (c *Controller) invalidateRegion(page Page) {
	if cached, ok := c.regions[page]; ok {
		cached.Invalidate()
	}
}
