package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import "unsafe"

// libsignal v0.101.0's cbindgen no longer emits the `#define SignalFOO_LEN n`
// constants that this package used to read sizes from, so the Go constants
// are now written out literally. That trades a compiler-enforced link to
// upstream for a hand-copied number, which would silently rot the next time
// a size changes upstream.
//
// The header does still emit a named array typedef per distinct size
// (SignalType_FixedArrayN_uint8_t), and every one of these sizes appears in
// a signature we call. Comparing against those restores the guarantee: each
// pair below is a bidirectional compile-time assertion, since a negative
// array length is a compile error. If upstream changes a size, this file
// stops compiling instead of corrupting a buffer at runtime.
//
// Keep one pair per constant. A constant with no matching assertion is a
// constant nothing is checking.
var (
	_ [unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{}) - SVRKeyLen]struct{}
	_ [SVRKeyLen - unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{}) - BackupKeyLen]struct{}
	_ [BackupKeyLen - unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray16_uint8_t{}) - BackupIDLen]struct{}
	_ [BackupIDLen - unsafe.Sizeof(C.SignalType_FixedArray16_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{}) - ProfileKeyLen]struct{}
	_ [ProfileKeyLen - unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray16_uint8_t{}) - AccessKeyLen]struct{}
	_ [AccessKeyLen - unsafe.Sizeof(C.SignalType_FixedArray16_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray64_uint8_t{}) - ProfileKeyVersionEncodedLen]struct{}
	_ [ProfileKeyVersionEncodedLen - unsafe.Sizeof(C.SignalType_FixedArray64_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray329_uint8_t{}) - ProfileKeyCredentialRequestLen]struct{}
	_ [ProfileKeyCredentialRequestLen - unsafe.Sizeof(C.SignalType_FixedArray329_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray473_uint8_t{}) - ProfileKeyCredentialRequestContextLen]struct{}
	_ [ProfileKeyCredentialRequestContextLen - unsafe.Sizeof(C.SignalType_FixedArray473_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray153_uint8_t{}) - ExpiringProfileKeyCredentialLen]struct{}
	_ [ExpiringProfileKeyCredentialLen - unsafe.Sizeof(C.SignalType_FixedArray153_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray497_uint8_t{}) - ExpiringProfileKeyCredentialResponseLen]struct{}
	_ [ExpiringProfileKeyCredentialResponseLen - unsafe.Sizeof(C.SignalType_FixedArray497_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray65_uint8_t{}) - ProfileKeyCiphertextLen]struct{}
	_ [ProfileKeyCiphertextLen - unsafe.Sizeof(C.SignalType_FixedArray65_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray97_uint8_t{}) - profileKeyCommitmentLen]struct{}
	_ [profileKeyCommitmentLen - unsafe.Sizeof(C.SignalType_FixedArray97_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{}) - GroupMasterKeyLen]struct{}
	_ [GroupMasterKeyLen - unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray289_uint8_t{}) - GroupSecretParamsLen]struct{}
	_ [GroupSecretParamsLen - unsafe.Sizeof(C.SignalType_FixedArray289_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray97_uint8_t{}) - GroupPublicParamsLen]struct{}
	_ [GroupPublicParamsLen - unsafe.Sizeof(C.SignalType_FixedArray97_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{}) - GroupIdentifierLen]struct{}
	_ [GroupIdentifierLen - unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray65_uint8_t{}) - UUIDCiphertextLen]struct{}
	_ [UUIDCiphertextLen - unsafe.Sizeof(C.SignalType_FixedArray65_uint8_t{})]struct{}

	_ [unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{}) - ZKRandomnessLen]struct{}
	_ [ZKRandomnessLen - unsafe.Sizeof(C.SignalType_FixedArray32_uint8_t{})]struct{}
)
