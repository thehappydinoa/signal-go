package libsignal

import (
	"runtime"
	"runtime/cgo"
	"unsafe"
)

type handleWrapper struct {
	h      cgo.Handle
	pinner runtime.Pinner
}

// savePointer stores v for retrieval from a C async callback. The returned
// uintptr is passed to C as context; the callback must call deletePointer.
func savePointer(v any) uintptr {
	w := &handleWrapper{h: cgo.NewHandle(v)}
	w.pinner.Pin(w)
	return uintptr(unsafe.Pointer(w))
}

func restorePointer(p uintptr) any {
	w := (*handleWrapper)(unsafe.Pointer(p))
	return w.h.Value()
}

func deletePointer(p uintptr) {
	w := (*handleWrapper)(unsafe.Pointer(p))
	w.pinner.Unpin()
	w.h.Delete()
}
