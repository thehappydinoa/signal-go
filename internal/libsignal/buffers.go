package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"unsafe"
)

// As of libsignal v0.101.0, cbindgen emits C string parameters as
// `const int8_t *` rather than `const char *` (SignalCStringPtr is now a
// typedef for the former). Both are 1-byte character pointers with
// identical ABI, but cgo treats *C.char and *C.int8_t as distinct Go
// types, so the boundary needs an explicit reinterpret.

// cStr reinterprets a C.CString allocation as the signed-char pointer
// libsignal expects. The caller still owns the allocation and must free
// the original *C.char.
func cStr(p *C.char) *C.int8_t {
	return (*C.int8_t)(unsafe.Pointer(p))
}

// goStr copies a NUL-terminated string returned by libsignal into a Go
// string. It does not free the C allocation.
func goStr(p C.SignalCStringPtr) string {
	return C.GoString((*C.char)(unsafe.Pointer(p)))
}

// goBytestringArrayFromC copies a libsignal BytestringArray and frees the
// Rust allocation.
func goBytestringArrayFromC(arr C.SignalBytestringArray) [][]byte {
	defer C.signal_free_bytestring_array(arr)

	if arr.bytes.base == nil || arr.lengths.base == nil || arr.lengths.length == 0 {
		return nil
	}

	count := int(arr.lengths.length)
	lengths := unsafe.Slice(arr.lengths.base, count)
	out := make([][]byte, 0, count)
	offset := 0
	allBytes := unsafe.Slice((*byte)(unsafe.Pointer(arr.bytes.base)), int(arr.bytes.length))
	for _, n := range lengths {
		ln := int(n)
		if offset+ln > len(allBytes) {
			break
		}
		chunk := make([]byte, ln)
		copy(chunk, allBytes[offset:offset+ln])
		out = append(out, chunk)
		offset += ln
	}
	return out
}
