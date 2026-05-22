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
// pointer is passed to C as context; the callback must call deletePointer.
func savePointer(v any) unsafe.Pointer {
	w := &handleWrapper{h: cgo.NewHandle(v)}
	w.pinner.Pin(w)
	return unsafe.Pointer(w)
}

func restorePointer(p unsafe.Pointer) any {
	w := (*handleWrapper)(p)
	return w.h.Value()
}

func deletePointer(p unsafe.Pointer) {
	w := (*handleWrapper)(p)
	w.pinner.Unpin()
	w.h.Delete()
}
