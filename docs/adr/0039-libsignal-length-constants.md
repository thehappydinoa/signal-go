# ADR 0039 — Hardcoded libsignal length constants, guarded by compile-time assertions

- Status: Accepted
- Date: 2026-08-17

## Context

`internal/libsignal` needs the byte lengths of libsignal's fixed-size
values — profile keys, group secret params, zkgroup credentials, backup
keys, randomness. Since [ADR 0004](./0004-libsignal-pin.md) these have
been read straight out of the vendored cbindgen header:

```go
const ProfileKeyLen = int(C.SignalPROFILE_KEY_LEN)
```

That was the right shape: the pinned header was the single source of
truth, and a size change upstream could not silently diverge from our
Go-side view of it.

The v0.101.0 bump removed that option. Upstream moved to a new cbindgen
generator, and the regenerated `signal_ffi.h` **no longer emits the
`#define SignalFOO_LEN n` constants at all** — all 17 that this package
depended on are gone. The values themselves did not change (verified
against `rust/zkgroup/src/common/constants.rs`,
`rust/account-keys/src/{lib,backup}.rs`, and `rust/zkcredential/src/lib.rs`
in the pinned tree); only the generator's output changed.

Getting a size wrong here is not a compile error at the C boundary — it
is a wrong-sized buffer handed to Rust code that will write into it.
That makes this a memory-safety question, not a stylistic one, so the
replacement needs to fail loudly rather than plausibly.

Options considered:

1. **Hardcode the values as Go constants.** Simple, but severs the link
   to upstream: a future bump that changes a size would compile happily
   and corrupt memory at runtime.
2. **Patch the constants back into the vendored header** during
   `scripts/build-libsignal.sh`. Keeps the C-side spelling, but means
   maintaining a hand-written patch against a generated file, and the
   patch itself is exactly as unverified as option 1.
3. **Parse the values out of the Rust sources at build time.** Most
   faithful, but adds a fragile source-scraping step to a build that
   must work from a pre-built artifact with no Rust tree present
   (ADR 0037).

## Decision

Take option 1 for the values, and buy back the lost guarantee separately.

The Go constants are written out literally, each with a comment naming
the upstream Rust source that defines it. On its own this would be
option 1's silent-rot problem, so it is paired with a mandatory
compile-time check.

The new generator still emits a named typedef per distinct array size:

```c
typedef uint8_t SignalType_FixedArray289_uint8_t[289];
```

and every size we care about appears in the signature of a function we
actually call — which is precisely why we needed the constant. So
`internal/libsignal/constants_assert.go` asserts each Go constant
against the matching typedef, in both directions, using zero-length
array declarations:

```go
_ [unsafe.Sizeof(C.SignalType_FixedArray289_uint8_t{}) - GroupSecretParamsLen]struct{}
_ [GroupSecretParamsLen - unsafe.Sizeof(C.SignalType_FixedArray289_uint8_t{})]struct{}
```

A negative array length is a compile error, so any divergence between
our constant and the header stops the build. This was verified by
deliberately perturbing `GroupSecretParamsLen` and confirming the build
fails.

**Every length constant must have an assertion pair.** A constant with
no assertion is a constant nothing is checking, and is indistinguishable
from option 1.

## Consequences

- The source of truth moves from the header's `#define`s to the header's
  array typedefs. Both are generated from the same Rust constants, so
  the guarantee is equivalent in strength — a size change upstream still
  breaks the build rather than corrupting memory.
- The failure mode is a compile error in `constants_assert.go` naming
  the offending constant, which is a clearer signal for the next bump
  than the type errors that would otherwise appear scattered across call
  sites.
- Adding a new fixed-size constant now takes two steps rather than one:
  declare it, and add its assertion pair. The bump-libsignal skill and
  the file's own header comment both call this out.
- If a future generator drops the `SignalType_FixedArrayN_uint8_t`
  typedefs too, this mechanism dies with them and option 3
  (parsing the Rust sources) becomes the remaining path. That would be a
  good moment to revisit rather than to improvise.
- The values are duplicated between Go and upstream Rust. That
  duplication is deliberate and is exactly what the assertions police;
  it is not a candidate for later "cleanup" that removes the checks.
