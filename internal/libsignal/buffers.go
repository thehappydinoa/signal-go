package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"unsafe"
)

// cInt8 reinterprets a *C.char (as produced by C.CString) as the *C.int8_t
// that libsignal v0.101.2's cbindgen output now uses for string parameters
// (previously *C.char / SignalCStringPtr, a source-level rename cbindgen
// made when standardizing on int8_t for C string bytes — same one-byte C
// integer representation on every platform we build for, so this is a
// pointer reinterpret, not a value conversion).
func cInt8(p *C.char) *C.int8_t {
	return (*C.int8_t)(unsafe.Pointer(p))
}

// cChar is the inverse of [cInt8], for libsignal out-params (e.g.
// SignalCStringPtr) that now surface as *C.int8_t.
func cChar(p *C.int8_t) *C.char {
	return (*C.char)(unsafe.Pointer(p))
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
