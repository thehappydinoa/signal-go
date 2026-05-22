package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

// TokioAsyncContext wraps libsignal's Rust tokio runtime for async FFI
// operations such as CDSI lookups.
type TokioAsyncContext struct {
	ptr *C.SignalTokioAsyncContext
}

// NewTokioAsyncContext creates a tokio runtime for libsignal-net async calls.
func NewTokioAsyncContext() (*TokioAsyncContext, error) {
	var out C.SignalMutPointerTokioAsyncContext
	if err := checkError(C.signal_tokio_async_context_new(&out)); err != nil {
		return nil, err
	}
	return &TokioAsyncContext{ptr: out.raw}, nil
}

// Close destroys the tokio runtime.
func (t *TokioAsyncContext) Close() {
	if t.ptr != nil {
		C.signal_tokio_async_context_destroy(C.SignalMutPointerTokioAsyncContext{raw: t.ptr})
		t.ptr = nil
	}
}

func (t *TokioAsyncContext) cPtr() C.SignalConstPointerTokioAsyncContext {
	return C.SignalConstPointerTokioAsyncContext{raw: t.ptr}
}
