//go:build !darwin

package ui

import (
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
)

// openImagePicker keeps the Fyne implementation for platforms without the
// macOS native picker.
func openImagePicker(parent fyne.Window, _ string, done func(io.ReadCloser, error)) {
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if reader == nil {
			done(nil, err)
			return
		}
		done(reader, err)
	}, parent)
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".png", ".jpg", ".jpeg", ".webp"}))
	fileDialog.Show()
}
