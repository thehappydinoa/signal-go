package libsignal

/*
#cgo CFLAGS: -I${SRCDIR}/include
// libsignal embeds BoringSSL (C++), so we link the C++ runtime explicitly.
#cgo linux LDFLAGS: -L${SRCDIR}/lib -lsignal_ffi -ldl -lm -lpthread -lstdc++ -Wl,--no-warn-execstack
#cgo darwin LDFLAGS: -L${SRCDIR}/lib -lsignal_ffi -framework Security -framework CoreFoundation -lc++

#include <stdlib.h>
#include "signal_ffi.h"
*/
import "C"
