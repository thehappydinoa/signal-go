# ADR 0039 — `signalgo_compat.h` shim for the libsignal v0.106 header

**Status:** Accepted  
**Date:** 2026-10-26

## Context

libsignal v0.106.0 replaced the cbindgen-generated `signal_ffi.h` with a new
generator. Relevant differences for `internal/libsignal/`:

- The `#define Signal*_LEN` size macros (profile key, group params, SVR key,
  …) are gone; sizes now only appear inside `SignalType_FixedArray<N>_uint8_t`
  typedefs.
- `<stdlib.h>` is no longer included, so `C.free` is not reachable via
  `signal_ffi.h` alone.
- C strings are `const int8_t *` (`SignalCStringPtr`) rather than `char *`,
  and fixed-size out/in buffers are `SignalType_FixedArray<N>_uint8_t *`
  instead of `uint8_t (*)[N]`.

## Decision

1. Add [`internal/libsignal/include/signalgo_compat.h`](../../internal/libsignal/include/signalgo_compat.h),
   included after `signal_ffi.h` in every cgo preamble. It re-declares the
   removed size macros (values copied from the v0.97.2 header) and includes
   `<stdlib.h>`. This keeps the Go-side `C.SignalFOO_LEN` references stable.
2. Cross the `char*` / `int8_t*` and array-pointer differences with explicit
   `unsafe.Pointer` casts at the call sites (identical layout, so no copies).
3. The bump-libsignal audit must also diff `signalgo_compat.h` against the
   `SignalType_FixedArray<N>` typedefs whenever the pin moves.

## Consequences

- If upstream resizes a type, the compat macro goes stale silently; the
  existing round-trip tests (zkgroup, profile key, backup key) catch a
  mismatch at test time, but the audit step above is the real guard.
- Alternative considered: replace the macros with Go constants. Rejected
  because array types in cgo (`[C.SignalX_LEN]C.uchar`) need the C symbol.
