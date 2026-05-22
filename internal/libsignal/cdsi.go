package libsignal

/*
#include "signal_ffi.h"

extern SignalFfiError *bridge_cdsi_lookup_new(
	SignalConstPointerTokioAsyncContext async_runtime,
	SignalConstPointerConnectionManager connection_manager,
	const char *username,
	const char *password,
	SignalConstPointerLookupRequest request,
	uintptr_t ctx
);
extern SignalFfiError *bridge_cdsi_lookup_complete(
	SignalConstPointerTokioAsyncContext async_runtime,
	SignalConstPointerCdsiLookup lookup,
	uintptr_t ctx
);
*/
import "C"

import (
	"fmt"
	"strconv"
	"time"
	"unsafe"
)

const cdsiLookupTimeout = 60 * time.Second

// CDSIResult is one phone-number lookup result from the CDSI enclave.
type CDSIResult struct {
	E164 uint64
	ACI  [16]byte
	PNI  [16]byte
}

// E164String formats the numeric E.164 as a +prefixed string.
func (r CDSIResult) E164String() string {
	if r.E164 == 0 {
		return ""
	}
	return "+" + strconv.FormatUint(r.E164, 10)
}

type cdsiLookupHandleResult struct {
	ptr *C.SignalCdsiLookup
	err error
}

type cdsiLookupResponseResult struct {
	entries []CDSIResult
	err     error
}

// CDSILookup performs a blocking CDSI lookup for the numbers in request
// using libsignal-net via the supplied tokio runtime and connection manager.
func CDSILookup(
	tokio *TokioAsyncContext,
	connMgr *ConnectionManager,
	username, password string,
	request *LookupRequest,
) ([]CDSIResult, error) {
	if tokio == nil || tokio.ptr == nil {
		return nil, fmt.Errorf("libsignal.CDSILookup: nil tokio context")
	}
	if connMgr == nil || connMgr.ptr == nil {
		return nil, fmt.Errorf("libsignal.CDSILookup: nil connection manager")
	}
	if request == nil || request.ptr == nil {
		return nil, fmt.Errorf("libsignal.CDSILookup: nil lookup request")
	}

	handle, err := cdsiLookupNew(tokio, connMgr, username, password, request)
	if err != nil {
		return nil, err
	}
	defer C.signal_cdsi_lookup_destroy(C.SignalMutPointerCdsiLookup{raw: handle})

	return cdsiLookupComplete(tokio, handle)
}

func cdsiLookupNew(
	tokio *TokioAsyncContext,
	connMgr *ConnectionManager,
	username, password string,
	request *LookupRequest,
) (*C.SignalCdsiLookup, error) {
	ch := make(chan cdsiLookupHandleResult, 1)
	ctx := savePointer(ch)

	cUser := C.CString(username)
	defer C.free(unsafe.Pointer(cUser))
	cPass := C.CString(password)
	defer C.free(unsafe.Pointer(cPass))

	if err := checkError(C.bridge_cdsi_lookup_new(
		tokio.cPtr(),
		connMgr.cPtr(),
		cUser,
		cPass,
		request.cPtr(),
		C.uintptr_t(ctx),
	)); err != nil {
		deletePointer(ctx)
		return nil, err
	}

	select {
	case result := <-ch:
		if result.err != nil {
			return nil, result.err
		}
		return result.ptr, nil
	case <-time.After(cdsiLookupTimeout):
		return nil, fmt.Errorf("libsignal.CDSILookup: timeout waiting for lookup handle")
	}
}

func cdsiLookupComplete(tokio *TokioAsyncContext, lookup *C.SignalCdsiLookup) ([]CDSIResult, error) {
	ch := make(chan cdsiLookupResponseResult, 1)
	ctx := savePointer(ch)

	if err := checkError(C.bridge_cdsi_lookup_complete(
		tokio.cPtr(),
		C.SignalConstPointerCdsiLookup{raw: lookup},
		C.uintptr_t(ctx),
	)); err != nil {
		deletePointer(ctx)
		return nil, err
	}

	select {
	case result := <-ch:
		return result.entries, result.err
	case <-time.After(cdsiLookupTimeout):
		return nil, fmt.Errorf("libsignal.CDSILookup: timeout waiting for lookup response")
	}
}

//export goCdsiLookupNewComplete
func goCdsiLookupNewComplete(errp *C.SignalFfiError, result *C.SignalCdsiLookup, ctx C.uintptr_t) {
	ch := restorePointer(uintptr(ctx)).(chan cdsiLookupHandleResult)
	deletePointer(uintptr(ctx))
	var r cdsiLookupHandleResult
	if errp != nil {
		r.err = checkError(errp)
	} else {
		r.ptr = result
	}
	ch <- r
}

//export goCdsiResponseComplete
func goCdsiResponseComplete(errp *C.SignalFfiError, result *C.SignalFfiCdsiLookupResponse, ctx C.uintptr_t) {
	ch := restorePointer(uintptr(ctx)).(chan cdsiLookupResponseResult)
	deletePointer(uintptr(ctx))
	var r cdsiLookupResponseResult
	if errp != nil {
		r.err = checkError(errp)
		ch <- r
		return
	}
	if result != nil && result.entries.base != nil && result.entries.length > 0 {
		entries := unsafe.Slice(result.entries.base, result.entries.length)
		r.entries = make([]CDSIResult, len(entries))
		for i := range entries {
			r.entries[i].E164 = uint64(entries[i].e164)
			for j := 0; j < 16; j++ {
				r.entries[i].ACI[j] = byte(entries[i].rawAciUuid[j])
				r.entries[i].PNI[j] = byte(entries[i].rawPniUuid[j])
			}
		}
	}
	ch <- r
}

// LookupToken reads the continuation token from an in-progress lookup.
// Call before destroying the lookup handle when saving state for delta lookups.
func LookupToken(lookup *C.SignalCdsiLookup) ([]byte, error) {
	if lookup == nil {
		return nil, fmt.Errorf("libsignal.LookupToken: nil lookup")
	}
	var out C.SignalOwnedBuffer
	if err := checkError(C.signal_cdsi_lookup_token(&out, C.SignalConstPointerCdsiLookup{raw: lookup})); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(out), nil
}
