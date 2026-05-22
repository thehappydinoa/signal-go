package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>

extern int signalgo_input_stream_read(void *ctx, size_t *out, SignalBorrowedMutableBuffer buf);
extern int signalgo_input_stream_skip(void *ctx, uint64_t amount);
extern void signalgo_input_stream_destroy(void *ctx);

static inline SignalInputStream signalgo_input_stream(void *ctx) {
    SignalInputStream s;
    s.ctx = ctx;
    s.read = signalgo_input_stream_read;
    s.skip = signalgo_input_stream_skip;
    s.destroy = signalgo_input_stream_destroy;
    return s;
}
*/
import "C"

import (
	"runtime"
	"runtime/cgo"
	"unsafe"
)

type bytesStreamState struct {
	data []byte
	off  int
}

// bytesInputStream exposes an in-memory byte slice to libsignal as a sync
// input stream. Each instance maintains its own read cursor.
type bytesInputStream struct {
	state   *bytesStreamState
	h       cgo.Handle
	ctxPtr  *uintptr
	cStream *C.SignalInputStream
}

func newBytesInputStream(data []byte) *bytesInputStream {
	state := &bytesStreamState{data: data}
	h := cgo.NewHandle(state)
	ctx := uintptr(h)
	ctxPtr := new(uintptr)
	*ctxPtr = ctx
	local := C.signalgo_input_stream(unsafe.Pointer(ctxPtr))
	cStream := (*C.SignalInputStream)(C.malloc(C.size_t(unsafe.Sizeof(local))))
	if cStream == nil {
		h.Delete()
		panic("libsignal: C.malloc failed for input stream")
	}
	// Copy in Go so we never pass a Go pointer into C.malloc's destination via C.
	copy(
		unsafe.Slice((*byte)(unsafe.Pointer(cStream)), int(unsafe.Sizeof(local))),
		unsafe.Slice((*byte)(unsafe.Pointer(&local)), int(unsafe.Sizeof(local))),
	)
	return &bytesInputStream{
		state:   state,
		h:       h,
		ctxPtr:  ctxPtr,
		cStream: cStream,
	}
}

func (s *bytesInputStream) ptr() C.SignalConstPointerFfiInputStreamStruct {
	return C.SignalConstPointerFfiInputStreamStruct{raw: s.cStream}
}

func (s *bytesInputStream) pin(p *runtime.Pinner) {
	p.Pin(s.ctxPtr)
	if len(s.state.data) > 0 {
		p.Pin(&s.state.data[0])
	}
}

func (s *bytesInputStream) release() {
	if s.cStream != nil {
		C.free(unsafe.Pointer(s.cStream))
		s.cStream = nil
	}
	if s.h != 0 {
		s.h.Delete()
		s.h = 0
	}
	s.ctxPtr = nil
}

//export signalgo_input_stream_read
func signalgo_input_stream_read(ctx unsafe.Pointer, out *C.size_t, buf C.SignalBorrowedMutableBuffer) C.int {
	if ctx == nil || out == nil {
		return C.int(1)
	}
	h := cgo.Handle(*(*uintptr)(ctx))
	state, ok := h.Value().(*bytesStreamState)
	if !ok {
		return C.int(1)
	}
	if buf.base == nil || buf.length == 0 {
		*out = 0
		return 0
	}
	remaining := state.data[state.off:]
	if len(remaining) == 0 {
		*out = 0
		return 0
	}
	n := int(buf.length)
	if n > len(remaining) {
		n = len(remaining)
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(buf.base)), n)
	copy(dst, remaining[:n])
	state.off += n
	*out = C.size_t(n)
	return 0
}

//export signalgo_input_stream_skip
func signalgo_input_stream_skip(ctx unsafe.Pointer, amount C.uint64_t) C.int {
	if ctx == nil {
		return C.int(1)
	}
	h := cgo.Handle(*(*uintptr)(ctx))
	state, ok := h.Value().(*bytesStreamState)
	if !ok {
		return C.int(1)
	}
	skip := int(amount)
	if skip < 0 || state.off+skip > len(state.data) {
		return C.int(1)
	}
	state.off += skip
	return 0
}

//export signalgo_input_stream_destroy
func signalgo_input_stream_destroy(_ unsafe.Pointer) {}
