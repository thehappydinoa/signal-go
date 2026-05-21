package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

// Address is a libsignal-owned ProtocolAddress (service id + device id).
type Address struct {
	raw C.SignalMutPointerProtocolAddress
}

// NewAddress allocates a fresh libsignal ProtocolAddress.
func NewAddress(serviceID string, deviceID uint32) (*Address, error) {
	if serviceID == "" {
		return nil, errors.New("libsignal.NewAddress: empty service id")
	}
	cname := C.CString(serviceID)
	defer C.free(unsafe.Pointer(cname))
	var out C.SignalMutPointerProtocolAddress
	if err := checkError(C.signal_address_new(&out, cname, C.uint32_t(deviceID))); err != nil {
		return nil, err
	}
	return wrapAddress(out), nil
}

// ServiceID returns the address's "name" string (ACI or PNI).
func (a *Address) ServiceID() (string, error) {
	var cstr *C.char
	if err := checkError(C.signal_address_get_name(&cstr, a.constPtr())); err != nil {
		return "", err
	}
	s := C.GoString(cstr)
	C.signal_free_string(cstr)
	return s, nil
}

// DeviceID returns the per-account device number.
func (a *Address) DeviceID() (uint32, error) {
	var out C.uint32_t
	if err := checkError(C.signal_address_get_device_id(&out, a.constPtr())); err != nil {
		return 0, err
	}
	return uint32(out), nil
}

func (a *Address) constPtr() C.SignalConstPointerProtocolAddress {
	return C.SignalConstPointerProtocolAddress{raw: a.raw.raw}
}

// rawMut exposes the libsignal-mutable pointer for callers that need to
// hand the address to another FFI call by value. The Address retains
// ownership and remains responsible for freeing the pointee.
func (a *Address) rawMut() C.SignalMutPointerProtocolAddress { return a.raw }

func wrapAddress(raw C.SignalMutPointerProtocolAddress) *Address {
	a := &Address{raw: raw}
	runtime.SetFinalizer(a, func(a *Address) {
		_ = checkError(C.signal_address_destroy(a.raw))
	})
	return a
}
