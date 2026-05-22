package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// ServiceIDFixedWidthLen is the libsignal fixed-width service id encoding size.
const ServiceIDFixedWidthLen = 17

// ServiceIDFixedWidth is libsignal's 17-byte service id wire form.
type ServiceIDFixedWidth [ServiceIDFixedWidthLen]byte

// ParseServiceIDString converts an ACI/PNI UUID string to fixed-width form.
func ParseServiceIDString(s string) (ServiceIDFixedWidth, error) {
	if s == "" {
		return ServiceIDFixedWidth{}, errors.New("libsignal.ParseServiceIDString: empty input")
	}
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	var out C.SignalServiceIdFixedWidthBinaryBytes
	if err := checkError(C.signal_service_id_parse_from_service_id_string(&out, cstr)); err != nil {
		return ServiceIDFixedWidth{}, err
	}
	return serviceIDFromC(out), nil
}

// ServiceIDString renders a fixed-width service id as its canonical string
// (e.g. "00000000-0000-0000-0000-000000000001").
func ServiceIDString(id ServiceIDFixedWidth) (string, error) {
	var cstr *C.char
	if err := checkError(C.signal_service_id_service_id_string(&cstr, cServiceID(id))); err != nil {
		return "", err
	}
	s := C.GoString(cstr)
	C.signal_free_string(cstr)
	return s, nil
}

// MustParseServiceIDString is like [ParseServiceIDString] but panics on error.
// Intended for tests with known-good UUID strings.
func MustParseServiceIDString(s string) ServiceIDFixedWidth {
	id, err := ParseServiceIDString(s)
	if err != nil {
		panic(fmt.Sprintf("libsignal.MustParseServiceIDString(%q): %v", s, err))
	}
	return id
}

func serviceIDFromC(out C.SignalServiceIdFixedWidthBinaryBytes) ServiceIDFixedWidth {
	var id ServiceIDFixedWidth
	for i := range id {
		id[i] = byte(out[i])
	}
	return id
}

func cServiceID(id ServiceIDFixedWidth) *[ServiceIDFixedWidthLen]C.uint8_t {
	return (*[ServiceIDFixedWidthLen]C.uint8_t)(unsafe.Pointer(&id[0]))
}
