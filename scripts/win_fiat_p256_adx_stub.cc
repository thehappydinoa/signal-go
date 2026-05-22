// BoringSSL's fiat_p256 ADX helpers are only assembled for Mach-O and ELF.
// MinGW (x86_64-pc-windows-gnu) emits COFF, so bcm.cc.obj references
// fiat_p256_adx_{mul,sqr} without providing them. Delegate to the portable
// fiat implementation (OPENSSL_NO_ASM disables the ADX fast path in-header).
//
// Built and appended to libsignal_ffi.a by scripts/build-libsignal.sh on Windows.

#define OPENSSL_NO_ASM 1
#define OPENSSL_STATIC 1

#include <stdint.h>

#include "third_party/fiat/p256_64.h"

extern "C" {

void fiat_p256_adx_mul(uint64_t out1[4], const uint64_t arg1[4],
                       const uint64_t arg2[4]) {
  fiat_p256_mul(out1, arg1, arg2);
}

void fiat_p256_adx_sqr(uint64_t out1[4], const uint64_t arg1[4]) {
  fiat_p256_square(out1, arg1);
}

}  // extern "C"
