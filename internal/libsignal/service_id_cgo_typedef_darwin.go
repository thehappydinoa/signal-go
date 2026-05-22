// Clang-style cgo: bridge helpers for the `const SignalServiceIdFixedWidthBinaryBytes *`
// parameter pattern. macOS's clang preserves the array typedef name in DWARF
// regardless of `const` qualification, so cgo emits Go-visible parameter type
// `*C.SignalServiceIdFixedWidthBinaryBytes` — distinct from the GCC-side
// `*[17]C.uint8_t`. See service_id_cgo_typedef_default.go for the GCC build.

//go:build darwin

package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import "unsafe"

// cServiceID converts a Go-side ServiceIDFixedWidth to the pointer type cgo
// emits for `const SignalServiceIdFixedWidthBinaryBytes *` on macOS.
func cServiceID(id ServiceIDFixedWidth) *C.SignalServiceIdFixedWidthBinaryBytes {
	return (*C.SignalServiceIdFixedWidthBinaryBytes)(unsafe.Pointer(&id[0]))
}

// cServiceIDPtr is a no-op cast on macOS — clang's cgo already exposes the
// typedef'd pointer type that const-input parameters expect.
func cServiceIDPtr(sid *C.SignalServiceIdFixedWidthBinaryBytes) *C.SignalServiceIdFixedWidthBinaryBytes {
	return sid
}
