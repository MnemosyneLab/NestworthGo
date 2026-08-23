package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const modalScreenMargin float32 = 24

// fittedModalSize keeps a dialog within the parent canvas while preserving a
// useful size on the normal desktop window. The content itself is responsible
// for scrolling when the fitted height is smaller than its minimum content.
func fittedModalSize(parent fyne.Window, preferred, minimum fyne.Size) fyne.Size {
	size := preferred.Max(minimum)
	if parent == nil {
		return size
	}

	available := parent.Canvas().Size()
	if available.Width <= 0 || available.Height <= 0 {
		return size
	}

	maxWidth := fyne.Max(minimum.Width, available.Width-2*modalScreenMargin)
	maxHeight := fyne.Max(minimum.Height, available.Height-2*modalScreenMargin)
	size.Width = fyne.Min(size.Width, maxWidth)
	size.Height = fyne.Min(size.Height, maxHeight)
	return size.Max(minimum)
}

func modalFormContent(form *widget.Form, minimumHeight float32) fyne.CanvasObject {
	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(0, minimumHeight))
	return container.NewPadded(scroll)
}

// newResponsiveFormDialog builds the same confirm/cancel contract as
// dialog.ShowForm, but keeps the form inside a padded vertical scroll area.
// Buttons are deliberately outside that area so they remain visible while a
// long form is scrolled.
func newResponsiveFormDialog(parent fyne.Window, title, confirm, dismiss string, items []*widget.FormItem, preferred, minimum fyne.Size, callback func(bool)) *dialog.CustomDialog {
	form := widget.NewForm(items...)
	save := widget.NewButtonWithIcon(confirm, fyneTheme.Current().Icon(fyneTheme.IconNameConfirm), nil)
	cancel := widget.NewButtonWithIcon(dismiss, fyneTheme.Current().Icon(fyneTheme.IconNameCancel), nil)
	if form.Validate() != nil {
		save.Disable()
	}

	var formDialog *dialog.CustomDialog
	cancel.OnTapped = func() {
		formDialog.Hide()
		if callback != nil {
			callback(false)
		}
	}
	save.OnTapped = func() {
		if form.Validate() != nil {
			return
		}
		formDialog.Hide()
		if callback != nil {
			callback(true)
		}
	}
	form.SetOnValidationChanged(func(err error) {
		if err != nil {
			save.Disable()
			return
		}
		save.Enable()
	})

	formDialog = dialog.NewCustom(title, dismiss, modalFormContent(form, minimum.Height), parent)
	formDialog.SetButtons([]fyne.CanvasObject{cancel, save})
	formDialog.Resize(fittedModalSize(parent, preferred, minimum))
	return formDialog
}

func showResponsiveForm(parent fyne.Window, title, confirm, dismiss string, items []*widget.FormItem, preferred, minimum fyne.Size, callback func(bool)) {
	newResponsiveFormDialog(parent, title, confirm, dismiss, items, preferred, minimum, callback).Show()
}

// showResponsiveBackendForm keeps decimal input visible until the application
// mutation succeeds. This is important for manual portfolio forms: a server
// or persistence error should not force the user to re-enter an exact value.
func showResponsiveBackendForm(c *Controller, title, confirm, dismiss string, items []*widget.FormItem, preferred, minimum fyne.Size, work func() error, success func()) {
	form := widget.NewForm(items...)
	save := widget.NewButtonWithIcon(confirm, fyneTheme.Current().Icon(fyneTheme.IconNameConfirm), nil)
	cancel := widget.NewButtonWithIcon(dismiss, fyneTheme.Current().Icon(fyneTheme.IconNameCancel), nil)
	errorLabel := widget.NewLabel("")
	errorLabel.Importance = widget.DangerImportance
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Hide()
	if form.Validate() != nil {
		save.Disable()
	}

	var formDialog *dialog.CustomDialog
	showError := func(err error) {
		if err == nil {
			errorLabel.SetText("")
			errorLabel.Hide()
			return
		}
		errorLabel.SetText(c.translator.TranslateError(err))
		errorLabel.Show()
		errorLabel.Refresh()
	}
	cancel.OnTapped = func() { formDialog.Hide() }
	save.OnTapped = func() {
		if form.Validate() != nil {
			return
		}
		save.Disable()
		showError(nil)
		runBackend(c, work, func(err error) {
			if err != nil {
				showError(err)
				save.Enable()
				return
			}
			formDialog.Hide()
			if success != nil {
				success()
			}
		})
	}
	form.SetOnValidationChanged(func(err error) {
		if err != nil {
			save.Disable()
			return
		}
		save.Enable()
	})

	content := container.NewBorder(errorLabel, nil, nil, nil, modalFormContent(form, minimum.Height))
	formDialog = dialog.NewCustom(title, dismiss, content, c.window)
	formDialog.SetButtons([]fyne.CanvasObject{cancel, save})
	formDialog.Resize(fittedModalSize(c.window, preferred, minimum))
	formDialog.Show()
}

func showResponsiveConfirm(parent fyne.Window, title, message string, callback func(bool)) {
	confirmDialog := dialog.NewConfirm(title, message, callback, parent)
	confirmDialog.Resize(fittedModalSize(parent, fyne.NewSize(520, 220), fyne.NewSize(400, 180)))
	confirmDialog.Show()
}
