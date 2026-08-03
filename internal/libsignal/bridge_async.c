#include "signal_ffi.h"

extern void goCdsiLookupNewComplete(SignalFfiError *err, SignalCdsiLookup *result, void *ctx);
extern void goCdsiResponseComplete(SignalFfiError *err, SignalFfiCdsiLookupResponse *result, void *ctx);

void bridge_cdsi_lookup_new_complete(SignalFfiError *err, const SignalMutPointerCdsiLookup *result, const void *ctx) {
    SignalCdsiLookup *raw = NULL;
    if (result != NULL) {
        raw = result->raw;
    }
    goCdsiLookupNewComplete(err, raw, (void *)ctx);
}

void bridge_cdsi_response_complete(SignalFfiError *err, const SignalFfiCdsiLookupResponse *result, const void *ctx) {
    goCdsiResponseComplete(err, (SignalFfiCdsiLookupResponse *)result, (void *)ctx);
}

SignalFfiError *bridge_cdsi_lookup_new(
    SignalConstPointerTokioAsyncContext async_runtime,
    SignalConstPointerConnectionManager connection_manager,
    const char *username,
    const char *password,
    SignalConstPointerLookupRequest request,
    void *ctx
) {
    SignalCPromiseMutPointerCdsiLookup promise = {0};
    promise.complete = bridge_cdsi_lookup_new_complete;
    promise.context = ctx;
    // signal_ffi.h (as of libsignal v0.99.3) takes int8_t* for string params
    // instead of char*; int8_t and char are the same width and bit pattern
    // for the ASCII/UTF-8 bytes CString() produces, so this reinterpretation
    // is safe. Keep this signature's own params as char* (matching
    // C.CString's return type) rather than propagating int8_t* to cdsi.go.
    return signal_cdsi_lookup_new(&promise, async_runtime, connection_manager, (const int8_t *)username, (const int8_t *)password, request);
}

SignalFfiError *bridge_cdsi_lookup_complete(
    SignalConstPointerTokioAsyncContext async_runtime,
    SignalConstPointerCdsiLookup lookup,
    void *ctx
) {
    SignalCPromiseFfiCdsiLookupResponse promise = {0};
    promise.complete = bridge_cdsi_response_complete;
    promise.context = ctx;
    return signal_cdsi_lookup_complete(&promise, async_runtime, lookup);
}
