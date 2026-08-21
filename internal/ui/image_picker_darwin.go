//go:build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#include <Cocoa/Cocoa.h>
#include <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <stdlib.h>

static char *nestworthPickImageFile(const char *prompt) {
	@autoreleasepool {
		NSOpenPanel *panel = [NSOpenPanel openPanel];
		panel.canChooseFiles = YES;
		panel.canChooseDirectories = NO;
		panel.allowsMultipleSelection = NO;
		panel.title = [NSString stringWithUTF8String:prompt != NULL ? prompt : "Set image"];
		panel.allowedContentTypes = @[
			[UTType typeWithIdentifier:@"public.png"],
			[UTType typeWithIdentifier:@"public.jpeg"],
			[UTType typeWithIdentifier:@"org.webmproject.webp"]
		];

		[NSApp activateIgnoringOtherApps:YES];
		if ([panel runModal] != NSModalResponseOK || panel.URL == nil) {
			return NULL;
		}
		return strdup(panel.URL.path.UTF8String);
	}
}
*/
import "C"

import (
	"io"
	"os"
	"unsafe"

	"fyne.io/fyne/v2"
)

// openImagePicker uses the native Cocoa file chooser on macOS. NSOpenPanel is
// modal and must be created on the Fyne main thread, which is also the Cocoa
// application thread for the desktop driver.
func openImagePicker(_ fyne.Window, title string, done func(io.ReadCloser, error)) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	path := C.nestworthPickImageFile(cTitle)
	if path == nil {
		done(nil, nil)
		return
	}
	selectedPath := C.GoString(path)
	C.free(unsafe.Pointer(path))

	reader, err := os.Open(selectedPath)
	done(reader, err)
}
