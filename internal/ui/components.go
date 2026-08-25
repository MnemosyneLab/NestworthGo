package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func surface(content fyne.CanvasObject, minimum fyne.Size) fyne.CanvasObject {
	background := canvas.NewRectangle(currentColor(fyneTheme.ColorNameInputBackground))
	background.CornerRadius = 16
	background.StrokeColor = currentColor(fyneTheme.ColorNameSeparator)
	background.StrokeWidth = 1
	background.SetMinSize(minimum)
	return container.NewStack(background, cardContent(content))
}

func sectionCard(title, description string, body fyne.CanvasObject) fyne.CanvasObject {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	titleLabel.SizeName = fyneTheme.SizeNameHeadingText

	descriptionLabel := mutedLabel(description)
	descriptionLabel.SizeName = fyneTheme.SizeNameText
	descriptionLabel.Wrapping = fyne.TextWrapWord

	header := container.New(layout.NewCustomPaddedVBoxLayout(8), titleLabel, descriptionLabel)
	content := container.New(layout.NewCustomPaddedVBoxLayout(12), header, separatorLine(), body)
	return settingsSurface(content, fyne.NewSize(1, 150))
}

func settingsSurface(content fyne.CanvasObject, minimum fyne.Size) fyne.CanvasObject {
	background := canvas.NewRectangle(currentColor(fyneTheme.ColorNameBackground))
	background.CornerRadius = 16
	background.StrokeColor = currentColor(fyneTheme.ColorNameSeparator)
	background.StrokeWidth = 1
	background.SetMinSize(minimum)
	return container.NewStack(background, cardContent(content))
}

func metricCard(title, value, detail string, accent color.Color) fyne.CanvasObject {
	valueLabel := canvas.NewText(value, accent)
	valueLabel.TextSize = 24
	valueLabel.TextStyle = fyne.TextStyle{Bold: true}
	content := container.NewVBox(
		mutedLabel(title),
		valueLabel,
		mutedLabel(detail),
	)
	// The detail copy is intentionally allowed to wrap. A 112px card clipped
	// the second line on the Investments page at the default desktop width.
	return surface(content, fyne.NewSize(1, 136))
}

func mutedLabel(value string) *widget.Label {
	label := widget.NewLabel(value)
	// LowImportance maps to Fyne's disabled foreground color, which is too
	// faint for explanatory copy on the light surface. Keep secondary text
	// readable and let the surrounding hierarchy provide the visual weight.
	label.Importance = widget.MediumImportance
	label.Wrapping = fyne.TextWrapWord
	return label
}

func sectionTitle(title, description string) fyne.CanvasObject {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	titleLabel.SizeName = fyneTheme.SizeNameHeadingText
	descriptionLabel := mutedLabel(description)
	descriptionLabel.Wrapping = fyne.TextWrapWord
	return container.New(layout.NewCustomPaddedVBoxLayout(8), titleLabel, descriptionLabel)
}

func badge(value string, fill, foreground color.Color) fyne.CanvasObject {
	label := canvas.NewText(value, foreground)
	label.TextSize = 12
	label.TextStyle = fyne.TextStyle{Bold: true}
	background := canvas.NewRectangle(fill)
	background.CornerRadius = 9
	background.SetMinSize(fyne.NewSize(label.MinSize().Width+20, label.MinSize().Height+10))
	return container.NewStack(background, container.NewPadded(label))
}

func keyValueRow(label, value string) fyne.CanvasObject {
	labelView := widget.NewLabel(label)
	labelView.Wrapping = fyne.TextWrapWord
	valueView := widget.NewLabelWithStyle(value, fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
	valueView.Wrapping = fyne.TextWrapWord
	return container.NewBorder(nil, nil, labelView, nil, valueView)
}

func settingsRows(objects ...fyne.CanvasObject) fyne.CanvasObject {
	withSeparators := make([]fyne.CanvasObject, 0, len(objects)*2)
	for index, object := range objects {
		if index > 0 {
			withSeparators = append(withSeparators, separatorLine())
		}
		withSeparators = append(withSeparators, object)
	}
	return container.New(layout.NewCustomPaddedVBoxLayout(10), withSeparators...)
}

func settingsRow(label string, field fyne.CanvasObject) fyne.CanvasObject {
	labelView := widget.NewLabelWithStyle(label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	labelView.SizeName = fyneTheme.SizeNameText
	labelView.Wrapping = fyne.TextWrapWord

	return container.NewBorder(nil, nil, labelView, settingsControl(field))
}

func settingsControl(content fyne.CanvasObject) fyne.CanvasObject {
	background := canvas.NewRectangle(currentColor(fyneTheme.ColorNameInputBackground))
	background.CornerRadius = 10
	background.StrokeColor = currentColor(fyneTheme.ColorNameSeparator)
	background.StrokeWidth = 1
	background.SetMinSize(fyne.NewSize(300, 44))
	return container.NewStack(background, controlContent(content))
}

const (
	cardContentPadding    float32 = 12
	controlContentPadding float32 = 8
)

func cardContent(content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(cardContentPadding, cardContentPadding, cardContentPadding, cardContentPadding), content)
}

func controlContent(content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(controlContentPadding, controlContentPadding, controlContentPadding, controlContentPadding), content)
}

func withMinSize(content fyne.CanvasObject, minimum fyne.Size) fyne.CanvasObject {
	background := canvas.NewRectangle(color.Transparent)
	background.SetMinSize(minimum)
	return container.NewStack(background, content)
}

func separatorLine() fyne.CanvasObject {
	line := canvas.NewRectangle(currentColor(fyneTheme.ColorNameSeparator))
	line.SetMinSize(fyne.NewSize(1, 1))
	return line
}

func compactVBox(objects ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewVBoxLayout(), objects...)
}
