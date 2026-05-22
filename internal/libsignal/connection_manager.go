package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import "unsafe"

const (
	// NetworkEnvironmentProduction is Signal's production service environment.
	NetworkEnvironmentProduction uint8 = 1
	// BuildVariantProduction is the production client build variant.
	BuildVariantProduction uint8 = 0
)

// ConnectionManager wraps libsignal-net's connection manager for CDSI and
// other libsignal-net network operations.
type ConnectionManager struct {
	ptr *C.SignalConnectionManager
}

// NewConnectionManager creates a connection manager for the given network
// environment and user agent string.
func NewConnectionManager(environment uint8, userAgent string) (*ConnectionManager, error) {
	var configMap C.SignalMutPointerBridgedStringMap
	if err := checkError(C.signal_bridged_string_map_new(&configMap, 0)); err != nil {
		return nil, err
	}
	cAgent := C.CString(userAgent)
	defer C.free(unsafe.Pointer(cAgent))

	var out C.SignalMutPointerConnectionManager
	if err := checkError(C.signal_connection_manager_new(
		&out,
		C.uint8_t(environment),
		cAgent,
		configMap,
		C.uint8_t(BuildVariantProduction),
	)); err != nil {
		C.signal_bridged_string_map_destroy(configMap)
		return nil, err
	}
	return &ConnectionManager{ptr: out.raw}, nil
}

// Close destroys the connection manager.
func (cm *ConnectionManager) Close() {
	if cm.ptr != nil {
		C.signal_connection_manager_destroy(C.SignalMutPointerConnectionManager{raw: cm.ptr})
		cm.ptr = nil
	}
}

func (cm *ConnectionManager) cPtr() C.SignalConstPointerConnectionManager {
	return C.SignalConstPointerConnectionManager{raw: cm.ptr}
}
