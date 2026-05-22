package libsignal

/*
#cgo CFLAGS: -I${SRCDIR}/include
// libsignal embeds BoringSSL (C++), so we link the C++ runtime explicitly.
// `-Wl,--no-warn-execstack` suppresses GNU ld's noisy warning about
// BoringSSL .S objects on Linux (also see scripts/build-libsignal.sh
// patch_gnu_stack); harmless if the section is already present.
#cgo linux LDFLAGS: -L${SRCDIR}/lib -lsignal_ffi -ldl -lm -lpthread -lstdc++ -Wl,--no-warn-execstack
#cgo darwin LDFLAGS: -L${SRCDIR}/lib -lsignal_ffi -framework Security -framework CoreFoundation -lc++
// Windows builds target the GNU (mingw-w64) toolchain so cgo can link
// the static archive produced by `cargo build --target x86_64-pc-windows-gnu`.
// The Win32 libraries listed below cover the syscalls that libsignal's
// transitive deps (rustls, boring-sys, tokio, hyper, ring, ...) reach
// for on Windows. The list mirrors the Rust ecosystem's typical "Win32
// link set" for staticlib targets; missing a library here surfaces as
// an `undefined reference` at Go link time.
#cgo windows LDFLAGS: -L${SRCDIR}/lib -lsignal_ffi -lws2_32 -luserenv -lbcrypt -ladvapi32 -lntdll -lkernel32 -luser32 -lcrypt32 -lsecur32 -lncrypt -lpsapi -liphlpapi -lstdc++ -lpthread -static

#include <stdlib.h>
#include "signal_ffi.h"
*/
import "C"
