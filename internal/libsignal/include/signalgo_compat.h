/*
 * signalgo_compat.h: constants and includes that signal_ffi.h used to provide
 * before libsignal v0.106.0 switched header generators. Values are the fixed
 * wire sizes of the corresponding libsignal types; keep in sync with the
 * SignalType_FixedArray<N>_uint8_t typedefs in signal_ffi.h.
 */
#ifndef SIGNALGO_COMPAT_H_
#define SIGNALGO_COMPAT_H_

#include <stdlib.h>

#define SignalSVR_KEY_LEN 32
#define SignalBACKUP_KEY_LEN 32
#define SignalGROUP_MASTER_KEY_LEN 32
#define SignalGROUP_SECRET_PARAMS_LEN 289
#define SignalGROUP_PUBLIC_PARAMS_LEN 97
#define SignalGROUP_IDENTIFIER_LEN 32
#define SignalPROFILE_KEY_LEN 32
#define SignalPROFILE_KEY_CIPHERTEXT_LEN 65
#define SignalPROFILE_KEY_COMMITMENT_LEN 97
#define SignalEXPIRING_PROFILE_KEY_CREDENTIAL_LEN 153
#define SignalPROFILE_KEY_CREDENTIAL_REQUEST_LEN 329
#define SignalPROFILE_KEY_CREDENTIAL_REQUEST_CONTEXT_LEN 473
#define SignalEXPIRING_PROFILE_KEY_CREDENTIAL_RESPONSE_LEN 497
#define SignalPROFILE_KEY_VERSION_ENCODED_LEN 64
#define SignalUUID_CIPHERTEXT_LEN 65
#define SignalRANDOMNESS_LEN 32
#define SignalACCESS_KEY_LEN 16

#endif /* SIGNALGO_COMPAT_H_ */
