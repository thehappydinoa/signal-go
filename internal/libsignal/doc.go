// Package libsignal is a Go wrapper around Signal's official Rust
// [libsignal] static library.
//
// All cryptographic operations used by signal-go (Curve25519 keys, X3DH /
// PQXDH session setup, Double Ratchet message encrypt/decrypt, Sealed
// Sender, zkgroup, profile cipher, attachment cipher, message backup keys,
// provisioning cipher) are delegated here. The package itself contains no
// crypto code — it converts Go arguments to the FFI ABI, calls into
// libsignal_ffi, converts results back, and frees Rust-owned memory.
//
// The build requires libsignal_ffi.a to exist at internal/libsignal/lib/.
// Run `task libsignal` from the repository root to produce it.
//
// # ABI conventions
//
//   - Every FFI function returns *SignalFfiError. Nil means success.
//   - Output parameters are passed as the last argument(s) and are written
//     iff the call succeeds.
//   - Rust-owned heap allocations come back as SignalOwnedBuffer (data +
//     length) or as opaque object pointers. Both must be freed by the
//     matching destroy/free function. We use [runtime.SetFinalizer] for
//     opaque objects and copy SignalOwnedBuffer contents out immediately.
//   - Borrowed buffers (SignalBorrowedBuffer) are read-only views the
//     callee will not retain past the call.
//
// [libsignal]: https://github.com/signalapp/libsignal
package libsignal
