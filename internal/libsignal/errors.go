package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// ErrorCode mirrors libsignal's SignalErrorCode enum. The numeric values
// come from the upstream header (search for `enum SignalErrorCode`).
type ErrorCode uint32

// Error is a libsignal FFI error converted to Go.
type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("libsignal: %s (code %d)", e.Message, e.Code)
}

// checkError consumes a *SignalFfiError. If non-nil, it pulls the message
// and type out, frees the error, and returns a Go error.
//
// The pointer is owned by libsignal until we free it here.
func checkError(rawErr *C.SignalFfiError) error {
	if rawErr == nil {
		return nil
	}
	// signal_error_get_type and signal_error_get_message both take a
	// *const SignalFfiError and never themselves return an error.
	code := ErrorCode(C.signal_error_get_type(rawErr))
	var cmsg C.SignalCStringPtr
	// signal_error_get_message returns a SignalFfiError* of its own if the
	// underlying error has no message; we ignore that and fall back.
	if e2 := C.signal_error_get_message(&cmsg, rawErr); e2 == nil && cmsg != nil {
		cstr := signalCStr(cmsg)
		msg := C.GoString(cstr)
		freeCStr(cstr)
		C.signal_error_free(rawErr)
		return &Error{Code: code, Message: msg}
	}
	C.signal_error_free(rawErr)
	return &Error{Code: code, Message: "(no message)"}
}

// goBytesFromOwnedBuffer copies the data out of a SignalOwnedBuffer and frees
// the underlying Rust allocation. Returns nil if buf is empty.
func goBytesFromOwnedBuffer(buf C.SignalOwnedBuffer) []byte {
	if buf.base == nil || buf.length == 0 {
		if buf.base != nil {
			C.signal_free_buffer(buf.base, buf.length)
		}
		return nil
	}
	out := C.GoBytes(unsafe.Pointer(buf.base), C.int(buf.length))
	C.signal_free_buffer(buf.base, buf.length)
	return out
}

// borrowed makes a SignalBorrowedBuffer pointing at the given slice. The
// caller must keep the slice alive across the FFI call.
//
//go:nosplit
func borrowed(b []byte) C.SignalBorrowedBuffer {
	if len(b) == 0 {
		return C.SignalBorrowedBuffer{}
	}
	return C.SignalBorrowedBuffer{
		base:   (*C.uchar)(unsafe.Pointer(&b[0])),
		length: C.size_t(len(b)),
	}
}

// keepAlive prevents the garbage collector from reclaiming b for the duration
// of the FFI call. cgo otherwise might not, since the pointer is hidden
// inside a C struct.
func keepAlive(b []byte) {
	runtime.KeepAlive(b)
}

// cStr converts a Go-owned C string (from C.CString) to the *int8_t type
// libsignal's FFI functions now declare for `const char *` parameters.
// cbindgen (as of the header shipped with libsignal v0.103.1) emits
// `int8_t` instead of the platform `char` type for byte-for-byte
// portability; the underlying bytes and calling convention are unchanged,
// so this is a plain pointer reinterpretation.
func cStr(s *C.char) *C.int8_t {
	return (*C.int8_t)(unsafe.Pointer(s))
}

// signalCStr converts a SignalCStringPtr (now `const int8_t *` in the
// header, previously `const char *`) returned by libsignal into a *C.char
// suitable for C.GoString.
func signalCStr(p C.SignalCStringPtr) *C.char {
	return (*C.char)(unsafe.Pointer(p))
}

// freeCStr releases a libsignal-allocated string obtained via signalCStr.
// signal_free_string's parameter is also now declared as int8_t* rather
// than char*.
func freeCStr(s *C.char) {
	C.signal_free_string(cStr(s))
}
