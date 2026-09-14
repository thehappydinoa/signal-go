package libsignal

/*
#include <stdlib.h>
#include "signal_ffi.h"
*/
import "C"

import (
	"unsafe"
)

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

// As of libsignal v0.102.0 the cbindgen surface emits C strings as `int8_t *`
// rather than `char *`, and returns them through the `SignalCStringPtr`
// typedef. cgo treats `*C.char` and `*C.int8_t` as distinct Go types, so every
// string crossing the boundary goes through these three helpers rather than
// being cast at each call site.

// cString allocates a NUL-terminated copy of s in the C heap, typed to match
// libsignal's `const int8_t *` string parameters. The caller must release it
// with freeCString.
func cString(s string) *C.int8_t {
	return (*C.int8_t)(unsafe.Pointer(C.CString(s)))
}

// freeCString releases a string allocated by cString.
func freeCString(p *C.int8_t) {
	C.free(unsafe.Pointer(p))
}

// goStringFromC copies a libsignal-owned C string into a Go string and frees
// the Rust allocation with signal_free_string. It returns "" for a nil pointer.
func goStringFromC(p C.SignalCStringPtr) string {
	if p == nil {
		return ""
	}
	defer C.signal_free_string(p)
	return C.GoString((*C.char)(unsafe.Pointer(p)))
}
