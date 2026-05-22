package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// ServiceIDFixedWidthLen is the byte length of a parsed service ID in
// libsignal's fixed-width binary format (1-byte kind tag + 16-byte UUID).
const ServiceIDFixedWidthLen = 17

// ParseServiceIDString converts an ACI/PNI UUID string (e.g.
// "9d0652a3-dcc3-4d11-975f-74d61598733f") into libsignal's 17-byte
// fixed-width service ID representation.
func ParseServiceIDString(serviceID string) ([ServiceIDFixedWidthLen]byte, error) {
	var out [ServiceIDFixedWidthLen]byte
	if serviceID == "" {
		return out, errors.New("libsignal.ParseServiceIDString: empty service id")
	}
	cstr := C.CString(serviceID)
	defer C.free(unsafe.Pointer(cstr))
	var sid C.SignalServiceIdFixedWidthBinaryBytes
	if err := checkError(C.signal_service_id_parse_from_service_id_string(&sid, cstr)); err != nil {
		return out, err
	}
	copy(out[:], C.GoBytes(unsafe.Pointer(&sid[0]), ServiceIDFixedWidthLen))
	return out, nil
}
