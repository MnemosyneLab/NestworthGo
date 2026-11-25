//go:build darwin && cgo

package preferences

/*
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>
#include <stdint.h>
char *nestworthPreferredLocale(void);
char *nestworthSystemTimezone(void);
int nestworthSystemDark(void);
void *nestworthWatchSystem(uint64_t token);
void nestworthUnwatchSystem(void *watch);
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

func nativeString(s *C.char) string {
	if s == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(s))
	return C.GoString(s)
}

func preferredLocale() string { return nativeString(C.nestworthPreferredLocale()) }
func SystemDark() bool        { return C.nestworthSystemDark() != 0 }

func SystemTimezone() string {
	if name := nativeString(C.nestworthSystemTimezone()); name != "" {
		return name
	}
	return "UTC"
}

var nextObserver atomic.Uint64
var observers sync.Map

// WatchSystemChanges delivers preference-change notifications outside the UI
// frame. The callback should request a redraw, leaving widget state to Gio.
func WatchSystemChanges(changed func()) func() {
	token := nextObserver.Add(1)
	observers.Store(token, changed)
	watch := C.nestworthWatchSystem(C.uint64_t(token))
	return sync.OnceFunc(func() {
		observers.Delete(token)
		C.nestworthUnwatchSystem(watch)
	})
}

//export nestworthSystemChanged
func nestworthSystemChanged(token C.uint64_t) {
	if changed, ok := observers.Load(uint64(token)); ok {
		changed.(func())()
	}
}
