//go:build darwin && !ios

package accessibility

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdint.h>
#include <stddef.h>

void nestworthAXAttach(uintptr_t handle, uintptr_t view);
void nestworthAXPublish(uintptr_t handle, const void *data, size_t length);
void nestworthAXDetach(uintptr_t handle);
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"gioui.org/app"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

var handles sync.Map
var nextHandle atomic.Uint64

// Bridge owns the Gio side of the AppKit accessibility adapter.
type Bridge struct {
	window  *app.Window
	viewRef uintptr
	frame   frame

	mu        sync.Mutex
	id        uintptr
	requested bool
	queued    request
}

func New(window *app.Window) *Bridge { return &Bridge{window: window} }

func (b *Bridge) View(event app.ViewEvent) {
	view, ok := event.(app.AppKitViewEvent)
	if !ok || view.View == b.viewRef {
		return
	}
	b.Close()
	if view.View == 0 {
		return
	}
	b.frame = frame{}
	b.viewRef = view.View
	id := uintptr(nextHandle.Add(1))
	b.mu.Lock()
	b.id = id
	b.mu.Unlock()
	handles.Store(id, b)
	C.nestworthAXAttach(C.uintptr_t(id), C.uintptr_t(view.View))
}

func (b *Bridge) Close() {
	b.mu.Lock()
	id := b.id
	b.id, b.requested, b.queued = 0, false, request{}
	b.mu.Unlock()
	if id == 0 {
		return
	}
	b.viewRef = 0
	handles.Delete(id)
	C.nestworthAXDetach(C.uintptr_t(id))
}

func (b *Bridge) Begin(gtx layout.Context) layout.Context {
	b.mu.Lock()
	active, queued := b.requested, b.queued
	b.queued = request{}
	b.mu.Unlock()
	if b.viewRef != 0 && (active || b.frame.last == nil) {
		return b.frame.start(gtx, queued)
	}
	return gtx
}

func (b *Bridge) Publish(gtx layout.Context) {
	if forContext(gtx) == nil {
		return
	}
	entries, changed := b.frame.snapshot(gtx)
	if !changed {
		return
	}
	data, err := jsoncodec.Marshal(entries)
	if err == nil {
		C.nestworthAXPublish(C.uintptr_t(b.id), unsafe.Pointer(unsafe.SliceData(data)), C.size_t(len(data)))
	}
}

//export nestworthAXRequest
func nestworthAXRequest(handle C.uintptr_t) {
	value, ok := handles.Load(uintptr(handle))
	if !ok {
		return
	}
	b := value.(*Bridge)
	b.mu.Lock()
	if b.id != uintptr(handle) {
		b.mu.Unlock()
		return
	}
	first := !b.requested
	b.requested = true
	b.mu.Unlock()
	// AppKit is querying on its main thread. Gio wakes that same thread.
	if first {
		go b.window.Invalidate()
	}
}

//export nestworthAXAction
func nestworthAXAction(handle C.uintptr_t, id C.uint64_t, kind C.uint8_t, value *C.char, start, length C.int64_t) C.int {
	bridge, ok := handles.Load(uintptr(handle))
	if !ok {
		return 0
	}
	b := bridge.(*Bridge)
	b.mu.Lock()
	if b.id != uintptr(handle) || b.queued.id != 0 {
		b.mu.Unlock()
		return 0
	}
	b.queued = request{id: uint64(id), action: action(kind), value: C.GoString(value), start: int(start), length: int(length)}
	b.mu.Unlock()
	go b.window.Invalidate()
	return 1
}
