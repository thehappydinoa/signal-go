package libsignal

// libsignal's cbindgen-generated signal_ffi.h stopped emitting the
// `#define SignalXXX_LEN N` integer macros as of v0.102.2 (the fixed-size
// output buffers are now expressed purely through named array typedefs like
// `SignalType_FixedArray32_uint8_t`, with no accompanying C constant). These
// mirror the removed macros so the rest of the package can keep sizing
// buffers by name instead of by magic number.
//
// Values are protocol-fixed cryptographic constants that do not change
// between libsignal releases; verified against the pinned v0.102.2 Rust
// source (rust/zkgroup/src/common/constants.rs,
// rust/account-keys/src/{lib,backup}.rs, rust/zkcredential/src/lib.rs) and
// against the removed macros in the previous (v0.97.2) header.
const (
	signalAccessKeyLen                            = 16
	signalBackupKeyLen                            = 32
	signalExpiringProfileKeyCredentialLen         = 153
	signalExpiringProfileKeyCredentialResponseLen = 497
	signalGroupIdentifierLen                      = 32
	signalGroupMasterKeyLen                       = 32
	signalGroupPublicParamsLen                    = 97
	signalGroupSecretParamsLen                    = 289
	signalProfileKeyCiphertextLen                 = 65
	signalProfileKeyCommitmentLen                 = 97
	signalProfileKeyCredentialRequestContextLen   = 473
	signalProfileKeyCredentialRequestLen          = 329
	signalProfileKeyLen                           = 32
	signalProfileKeyVersionEncodedLen             = 64
	signalRandomnessLen                           = 32
	signalSVRKeyLen                               = 32
	signalUUIDCiphertextLen                       = 65
)
