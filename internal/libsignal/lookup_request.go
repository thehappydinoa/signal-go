package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import "unsafe"

// LookupRequest is a libsignal CDSI phone-number lookup request.
type LookupRequest struct {
	ptr *C.SignalLookupRequest
}

// NewLookupRequest creates an empty CDSI lookup request.
func NewLookupRequest() (*LookupRequest, error) {
	var out C.SignalMutPointerLookupRequest
	if err := checkError(C.signal_lookup_request_new(&out)); err != nil {
		return nil, err
	}
	return &LookupRequest{ptr: out.raw}, nil
}

// AddE164 adds an E.164 phone number (e.g. "+15551234567") to the request.
func (r *LookupRequest) AddE164(e164 string) error {
	cstr := C.CString(e164)
	defer C.free(unsafe.Pointer(cstr))
	return checkError(C.signal_lookup_request_add_e164(r.cPtr(), cstr))
}

// AddPreviousE164 adds a previously looked-up E.164 for delta/token reuse.
func (r *LookupRequest) AddPreviousE164(e164 string) error {
	cstr := C.CString(e164)
	defer C.free(unsafe.Pointer(cstr))
	return checkError(C.signal_lookup_request_add_previous_e164(r.cPtr(), cstr))
}

// SetToken sets the continuation token from a prior lookup response.
func (r *LookupRequest) SetToken(token []byte) error {
	buf := borrowed(token)
	err := checkError(C.signal_lookup_request_set_token(r.cPtr(), buf))
	keepAlive(token)
	return err
}

// Close destroys the underlying lookup request.
func (r *LookupRequest) Close() {
	if r.ptr != nil {
		C.signal_lookup_request_destroy(C.SignalMutPointerLookupRequest{raw: r.ptr})
		r.ptr = nil
	}
}

func (r *LookupRequest) cPtr() C.SignalConstPointerLookupRequest {
	return C.SignalConstPointerLookupRequest{raw: r.ptr}
}
