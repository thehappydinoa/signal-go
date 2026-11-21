package libsignal

/*
#include <stdlib.h>
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// UnidentifiedAccessKeyLen is the fixed byte length of an
// UnidentifiedAccessKey required by RegistrationAccountAttributes.
const UnidentifiedAccessKeyLen = 16

// RegistrationAccountAttributesOptions bundles the inputs to
// [NewRegistrationAccountAttributes]. RecoveryPassword is the SVR
// master-key-derived recovery password; UnidentifiedAccessKey must be
// exactly 16 bytes; Capabilities is an opaque list of capability flag
// strings the server expects.
type RegistrationAccountAttributesOptions struct {
	RecoveryPassword               []byte
	ACIRegistrationID              uint16
	PNIRegistrationID              uint16
	RegistrationLock               string
	UnidentifiedAccessKey          []byte
	UnrestrictedUnidentifiedAccess bool
	Capabilities                   []string
	DiscoverableByPhoneNumber      bool
}

// RegistrationAccountAttributes is a libsignal account-attributes handle
// used by register_account_without_number. Close is idempotent.
type RegistrationAccountAttributes struct {
	ptr *C.SignalRegistrationAccountAttributes
}

// NewRegistrationAccountAttributes allocates an attributes handle from the
// given options. The UnidentifiedAccessKey must be exactly
// [UnidentifiedAccessKeyLen] bytes; all other buffers may be empty.
func NewRegistrationAccountAttributes(opts RegistrationAccountAttributesOptions) (*RegistrationAccountAttributes, error) {
	if len(opts.UnidentifiedAccessKey) != UnidentifiedAccessKeyLen {
		return nil, errors.New("libsignal.NewRegistrationAccountAttributes: UnidentifiedAccessKey must be 16 bytes")
	}

	cLock := C.CString(opts.RegistrationLock)
	defer C.free(unsafe.Pointer(cLock))

	var uak [UnidentifiedAccessKeyLen]C.uint8_t
	for i, b := range opts.UnidentifiedAccessKey {
		uak[i] = C.uint8_t(b)
	}

	caps, freeCaps := bytestringArrayFromStrings(opts.Capabilities)
	defer freeCaps()

	var out C.SignalMutPointerRegistrationAccountAttributes
	err := checkError(C.signal_registration_account_attributes_create(
		&out,
		borrowed(opts.RecoveryPassword),
		C.uint16_t(opts.ACIRegistrationID),
		C.uint16_t(opts.PNIRegistrationID),
		(*C.int8_t)(unsafe.Pointer(cLock)),
		&uak,
		C.bool(opts.UnrestrictedUnidentifiedAccess),
		caps,
		C.bool(opts.DiscoverableByPhoneNumber),
	))
	keepAlive(opts.RecoveryPassword)
	keepAlive(opts.UnidentifiedAccessKey)
	if err != nil {
		return nil, err
	}
	return &RegistrationAccountAttributes{ptr: out.raw}, nil
}

// Close destroys the attributes handle. Idempotent.
func (a *RegistrationAccountAttributes) Close() {
	if a == nil || a.ptr == nil {
		return
	}
	C.signal_registration_account_attributes_destroy(C.SignalMutPointerRegistrationAccountAttributes{raw: a.ptr})
	a.ptr = nil
}

// bytestringArrayFromStrings packs []string into a SignalBorrowedBytestringArray
// layout. The returned free func must run after the FFI call that consumes
// the array, and the caller must keep the strings alive across that call.
func bytestringArrayFromStrings(items []string) (C.SignalBorrowedBytestringArray, func()) {
	if len(items) == 0 {
		return C.SignalBorrowedBytestringArray{}, func() {}
	}
	// Concatenate into one buffer + a lengths slice so Rust sees the
	// borrowed bytestring array layout.
	var total int
	for _, s := range items {
		total += len(s)
	}
	buf := C.malloc(C.size_t(total))
	lens := C.malloc(C.size_t(len(items)) * C.size_t(unsafe.Sizeof(C.size_t(0))))
	lenSlice := unsafe.Slice((*C.size_t)(lens), len(items))
	bufSlice := unsafe.Slice((*byte)(buf), total)

	offset := 0
	for i, s := range items {
		copy(bufSlice[offset:offset+len(s)], s)
		lenSlice[i] = C.size_t(len(s))
		offset += len(s)
	}

	arr := C.SignalBorrowedBytestringArray{
		bytes: C.SignalBorrowedBuffer{
			base:   (*C.uchar)(buf),
			length: C.size_t(total),
		},
		lengths: C.SignalBorrowedSliceOfusize{
			base:   (*C.size_t)(lens),
			length: C.size_t(len(items)),
		},
	}
	return arr, func() {
		C.free(buf)
		C.free(lens)
	}
}
