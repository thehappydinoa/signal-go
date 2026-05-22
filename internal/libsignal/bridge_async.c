#include "signal_ffi.h"

extern void goCdsiLookupNewComplete(SignalFfiError *err, SignalCdsiLookup *result, uintptr_t ctx);
extern void goCdsiResponseComplete(SignalFfiError *err, SignalFfiCdsiLookupResponse *result, uintptr_t ctx);

void bridge_cdsi_lookup_new_complete(SignalFfiError *err, const SignalMutPointerCdsiLookup *result, const void *ctx) {
    SignalCdsiLookup *raw = NULL;
    if (result != NULL) {
        raw = result->raw;
    }
    goCdsiLookupNewComplete(err, raw, (uintptr_t)ctx);
}

void bridge_cdsi_response_complete(SignalFfiError *err, const SignalFfiCdsiLookupResponse *result, const void *ctx) {
    goCdsiResponseComplete(err, (SignalFfiCdsiLookupResponse *)result, (uintptr_t)ctx);
}

SignalFfiError *bridge_cdsi_lookup_new(
    SignalConstPointerTokioAsyncContext async_runtime,
    SignalConstPointerConnectionManager connection_manager,
    const char *username,
    const char *password,
    SignalConstPointerLookupRequest request,
    uintptr_t ctx
) {
    SignalCPromiseMutPointerCdsiLookup promise = {0};
    promise.complete = bridge_cdsi_lookup_new_complete;
    promise.context = (const void *)ctx;
    return signal_cdsi_lookup_new(&promise, async_runtime, connection_manager, username, password, request);
}

SignalFfiError *bridge_cdsi_lookup_complete(
    SignalConstPointerTokioAsyncContext async_runtime,
    SignalConstPointerCdsiLookup lookup,
    uintptr_t ctx
) {
    SignalCPromiseFfiCdsiLookupResponse promise = {0};
    promise.complete = bridge_cdsi_response_complete;
    promise.context = (const void *)ctx;
    return signal_cdsi_lookup_complete(&promise, async_runtime, lookup);
}
