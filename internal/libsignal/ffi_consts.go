package libsignal

// Fixed byte lengths of libsignal types. Since libsignal v0.104 the generated
// signal_ffi.h no longer exports these as #defines (only as FixedArrayN
// typedefs), so they are mirrored here. A mismatch fails the build: the
// array-typed FFI parameters are checked against the header's typedefs by cgo.
const (
	ffiAccessKeyLen                            = 16
	ffiBackupKeyLen                            = 32
	ffiExpiringProfileKeyCredentialLen         = 153
	ffiExpiringProfileKeyCredentialResponseLen = 497
	ffiGroupIdentifierLen                      = 32
	ffiGroupMasterKeyLen                       = 32
	ffiGroupPublicParamsLen                    = 97
	ffiGroupSecretParamsLen                    = 289
	ffiProfileKeyCiphertextLen                 = 65
	ffiProfileKeyCommitmentLen                 = 97
	ffiProfileKeyCredentialRequestContextLen   = 473
	ffiProfileKeyCredentialRequestLen          = 329
	ffiProfileKeyLen                           = 32
	ffiProfileKeyVersionEncodedLen             = 64
	ffiRandomnessLen                           = 32
	ffiSvrKeyLen                               = 32
	ffiUUIDCiphertextLen                       = 65
)
