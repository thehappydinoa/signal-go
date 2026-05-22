package libsignal

/*
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
