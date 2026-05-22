// GCC-style cgo: bridge helpers for the `const SignalServiceIdFixedWidthBinaryBytes *`
// parameter pattern. On GCC-derived host C toolchains (Linux glibc, Windows
// MinGW-w64, FreeBSD) cgo unwraps the array typedef when reading DWARF for a
// `const`-qualified pointer parameter — so the Go-visible signature uses
// `*[17]C.uint8_t`, not `*C.SignalServiceIdFixedWidthBinaryBytes`. The macOS
// counterpart in service_id_cgo_typedef_darwin.go keeps the typedef name; both
// branches expose the same function names so call sites stay portable.

//go:build !darwin

package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import "unsafe"

// cServiceID converts a Go-side ServiceIDFixedWidth to the pointer type cgo
// emits for `const SignalServiceIdFixedWidthBinaryBytes *` on this toolchain.
// The id argument is a value parameter; Go escape analysis pins it for the
// duration of the cgo call, which is all we need.
func cServiceID(id ServiceIDFixedWidth) *[ServiceIDFixedWidthLen]C.uint8_t {
	return (*[ServiceIDFixedWidthLen]C.uint8_t)(unsafe.Pointer(&id[0]))
}

// cServiceIDPtr reinterprets a C-side `SignalServiceIdFixedWidthBinaryBytes`
// (typically produced by signal_service_id_parse_*) as the cgo-expected
// `const`-input pointer type. No copy; lifetime is the caller's responsibility.
func cServiceIDPtr(sid *C.SignalServiceIdFixedWidthBinaryBytes) *[ServiceIDFixedWidthLen]C.uint8_t {
	return (*[ServiceIDFixedWidthLen]C.uint8_t)(unsafe.Pointer(sid))
}
