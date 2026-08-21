package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestFittedModalSizeRespectsParentCanvas(t *testing.T) {
	application := test.NewTempApp(t)
	defer application.Quit()
	window := test.NewTempWindow(t, container.NewVBox())
	window.Resize(fyne.NewSize(640, 480))

	got := fittedModalSize(window, fyne.NewSize(720, 680), fyne.NewSize(560, 360))
	want := fyne.NewSize(592, 432)
	if got != want {
		t.Fatalf("fitted modal size = %v, want %v", got, want)
	}
}

func TestModalFormContentUsesPaddedVerticalScroll(t *testing.T) {
	form := widget.NewForm(widget.NewFormItem("Name", widget.NewEntry()))
	content := modalFormContent(form, 240)

	padded, ok := content.(*fyne.Container)
	if !ok || len(padded.Objects) != 1 {
		t.Fatalf("modal content = %T with %d children, want padded container with one child", content, len(padded.Objects))
	}
	scroll, ok := padded.Objects[0].(*container.Scroll)
	if !ok {
		t.Fatalf("padded child = %T, want *container.Scroll", padded.Objects[0])
	}
	if scroll.Direction != container.ScrollVerticalOnly {
		t.Fatalf("scroll direction = %v, want vertical-only", scroll.Direction)
	}
	if scroll.MinSize().Height < 240 {
		t.Fatalf("scroll minimum height = %v, want at least 240", scroll.MinSize().Height)
	}
}
