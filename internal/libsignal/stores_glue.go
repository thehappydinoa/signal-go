package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>

// Forward declarations of the //export'd Go functions in stores.go.
// We cannot include "_cgo_export.h" directly because this file is
// compiled before that header is generated.

extern int  signalgo_load_session(void*, SignalMutPointerSessionRecord*, SignalMutPointerProtocolAddress);
extern int  signalgo_store_session(void*, SignalMutPointerProtocolAddress, SignalMutPointerSessionRecord);

extern int  signalgo_get_local_identity_key_pair(void*, SignalPairOfMutPointerPrivateKeyMutPointerPublicKey*);
extern int  signalgo_get_local_registration_id(void*, uint32_t*);
extern int  signalgo_get_identity_key(void*, SignalMutPointerPublicKey*, SignalMutPointerProtocolAddress);
extern int  signalgo_save_identity_key(void*, uint8_t*, SignalMutPointerProtocolAddress, SignalMutPointerPublicKey);
extern int  signalgo_is_trusted_identity(void*, bool*, SignalMutPointerProtocolAddress, SignalMutPointerPublicKey, uint32_t);

extern int  signalgo_load_pre_key(void*, SignalMutPointerPreKeyRecord*, uint32_t);
extern int  signalgo_store_pre_key(void*, uint32_t, SignalMutPointerPreKeyRecord);
extern int  signalgo_remove_pre_key(void*, uint32_t);

extern int  signalgo_load_signed_pre_key(void*, SignalMutPointerSignedPreKeyRecord*, uint32_t);
extern int  signalgo_store_signed_pre_key(void*, uint32_t, SignalMutPointerSignedPreKeyRecord);

extern int  signalgo_load_kyber_pre_key(void*, SignalMutPointerKyberPreKeyRecord*, uint32_t);
extern int  signalgo_store_kyber_pre_key(void*, uint32_t, SignalMutPointerKyberPreKeyRecord);
extern int  signalgo_mark_kyber_pre_key_used(void*, uint32_t, uint32_t, SignalMutPointerPublicKey);

extern int  signalgo_load_sender_key(void*, SignalMutPointerSenderKeyRecord*, SignalMutPointerProtocolAddress, SignalUuid);
extern int  signalgo_store_sender_key(void*, SignalMutPointerProtocolAddress, SignalUuid, SignalMutPointerSenderKeyRecord);

extern void signalgo_destroy_store_handle(void*);

// Builders bundle the function pointers into the structs libsignal
// expects. We declare them static inline so each translation unit gets
// its own copy — keeps the symbol table clean.

static inline SignalSessionStore signalgo_session_store(void) {
    SignalSessionStore s;
    s.load_session  = signalgo_load_session;
    s.store_session = signalgo_store_session;
    s.destroy       = signalgo_destroy_store_handle;
    return s;
}

static inline SignalIdentityKeyStore signalgo_identity_key_store(void) {
    SignalIdentityKeyStore s;
    s.get_local_identity_key_pair = signalgo_get_local_identity_key_pair;
    s.get_local_registration_id   = signalgo_get_local_registration_id;
    s.get_identity_key            = signalgo_get_identity_key;
    s.save_identity_key           = signalgo_save_identity_key;
    s.is_trusted_identity         = signalgo_is_trusted_identity;
    s.destroy                     = signalgo_destroy_store_handle;
    return s;
}

static inline SignalPreKeyStore signalgo_pre_key_store(void) {
    SignalPreKeyStore s;
    s.load_pre_key   = signalgo_load_pre_key;
    s.store_pre_key  = signalgo_store_pre_key;
    s.remove_pre_key = signalgo_remove_pre_key;
    s.destroy        = signalgo_destroy_store_handle;
    return s;
}

static inline SignalSignedPreKeyStore signalgo_signed_pre_key_store(void) {
    SignalSignedPreKeyStore s;
    s.load_signed_pre_key  = signalgo_load_signed_pre_key;
    s.store_signed_pre_key = signalgo_store_signed_pre_key;
    s.destroy              = signalgo_destroy_store_handle;
    return s;
}

static inline SignalKyberPreKeyStore signalgo_kyber_pre_key_store(void) {
    SignalKyberPreKeyStore s;
    s.load_kyber_pre_key       = signalgo_load_kyber_pre_key;
    s.store_kyber_pre_key      = signalgo_store_kyber_pre_key;
    s.mark_kyber_pre_key_used  = signalgo_mark_kyber_pre_key_used;
    s.destroy                  = signalgo_destroy_store_handle;
    return s;
}

static inline SignalSenderKeyStore signalgo_sender_key_store(void) {
    SignalSenderKeyStore s;
    s.load_sender_key  = signalgo_load_sender_key;
    s.store_sender_key = signalgo_store_sender_key;
    s.destroy          = signalgo_destroy_store_handle;
    return s;
}
*/
import "C"

import (
	"runtime"
	"runtime/cgo"
	"unsafe"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// StoreHandle pins a Go store implementation against libsignal callbacks
// and yields the C struct(s) those callbacks expect. The caller is
// responsible for invoking [StoreHandle.Release] when the FFI call(s) are
// finished.
//
// One StoreHandle backs one Go store; if you have separate ACI- and
// PNI-namespace stores, register two handles.
type StoreHandle struct {
	value any
	h     cgo.Handle
	// ctxPtr holds the handle integer at a stable heap address so we can
	// hand libsignal a real pointer without tripping checkptr under -race.
	ctxPtr *uintptr

	// Go-side templates; copied into C.malloc'd blobs for FFI calls.
	session  C.SignalSessionStore
	identity C.SignalIdentityKeyStore
	preKey   C.SignalPreKeyStore
	signed   C.SignalSignedPreKeyStore
	kyber    C.SignalKyberPreKeyStore
	sender   C.SignalSenderKeyStore

	cSession  *C.SignalSessionStore
	cIdentity *C.SignalIdentityKeyStore
	cPreKey   *C.SignalPreKeyStore
	cSigned   *C.SignalSignedPreKeyStore
	cKyber    *C.SignalKyberPreKeyStore
}

// NewStoreHandle registers any store implementation through a
// [runtime/cgo.Handle]. The handle can then be used to materialise the
// libsignal callback structs.
func NewStoreHandle(s any) *StoreHandle {
	h := cgo.NewHandle(s)
	ctx := new(uintptr)
	*ctx = uintptr(h)
	sh := &StoreHandle{value: s, h: h, ctxPtr: ctx}
	ctxPtr := sh.Ctx()
	sh.session = SessionStoreFor(sh)
	sh.session.ctx = ctxPtr
	sh.identity = IdentityKeyStoreFor(sh)
	sh.identity.ctx = ctxPtr
	sh.preKey = PreKeyStoreFor(sh)
	sh.preKey.ctx = ctxPtr
	sh.signed = SignedPreKeyStoreFor(sh)
	sh.signed.ctx = ctxPtr
	sh.kyber = KyberPreKeyStoreFor(sh)
	sh.kyber.ctx = ctxPtr
	sh.sender = SenderKeyStoreFor(sh)
	sh.sender.ctx = ctxPtr
	sh.allocCStores()
	return sh
}

func (h *StoreHandle) allocCStores() {
	h.cSession = allocSessionStoreCopy(&h.session)
	h.cIdentity = allocIdentityStoreCopy(&h.identity)
	h.cPreKey = allocPreKeyStoreCopy(&h.preKey)
	h.cSigned = allocSignedStoreCopy(&h.signed)
	h.cKyber = allocKyberStoreCopy(&h.kyber)
}

func allocSessionStoreCopy(src *C.SignalSessionStore) *C.SignalSessionStore {
	return (*C.SignalSessionStore)(allocCopy(unsafe.Pointer(src), C.size_t(unsafe.Sizeof(*src))))
}

func allocIdentityStoreCopy(src *C.SignalIdentityKeyStore) *C.SignalIdentityKeyStore {
	return (*C.SignalIdentityKeyStore)(allocCopy(unsafe.Pointer(src), C.size_t(unsafe.Sizeof(*src))))
}

func allocPreKeyStoreCopy(src *C.SignalPreKeyStore) *C.SignalPreKeyStore {
	return (*C.SignalPreKeyStore)(allocCopy(unsafe.Pointer(src), C.size_t(unsafe.Sizeof(*src))))
}

func allocSignedStoreCopy(src *C.SignalSignedPreKeyStore) *C.SignalSignedPreKeyStore {
	return (*C.SignalSignedPreKeyStore)(allocCopy(unsafe.Pointer(src), C.size_t(unsafe.Sizeof(*src))))
}

func allocKyberStoreCopy(src *C.SignalKyberPreKeyStore) *C.SignalKyberPreKeyStore {
	return (*C.SignalKyberPreKeyStore)(allocCopy(unsafe.Pointer(src), C.size_t(unsafe.Sizeof(*src))))
}

func allocCopy(src unsafe.Pointer, size C.size_t) unsafe.Pointer {
	n := int(size)
	dst := C.malloc(size)
	if dst == nil {
		panic("libsignal: C.malloc failed")
	}
	// Copy in Go so we never pass a Go pointer into a C function.
	copy(unsafe.Slice((*byte)(dst), n), unsafe.Slice((*byte)(src), n))
	return dst
}

// Ctx returns the void* libsignal expects in the callback ctx slot.
func (h *StoreHandle) Ctx() unsafe.Pointer { return unsafe.Pointer(h.ctxPtr) }

// pinForFFI pins every field that crosses the cgo boundary (store structs
// embed Go pointers in their ctx slots).
func (h *StoreHandle) pinForFFI(p *runtime.Pinner) {
	p.Pin(h.ctxPtr)
}

// Release frees the cgo handle. Idempotent.
func (h *StoreHandle) Release() {
	if h == nil || h.h == 0 {
		return
	}
	h.freeCStores()
	h.h.Delete()
	h.h = 0
}

func (h *StoreHandle) freeCStores() {
	freeIfSet(unsafe.Pointer(h.cSession))
	freeIfSet(unsafe.Pointer(h.cIdentity))
	freeIfSet(unsafe.Pointer(h.cPreKey))
	freeIfSet(unsafe.Pointer(h.cSigned))
	freeIfSet(unsafe.Pointer(h.cKyber))
	h.cSession, h.cIdentity, h.cPreKey, h.cSigned, h.cKyber = nil, nil, nil, nil, nil
}

func freeIfSet(p unsafe.Pointer) {
	if p != nil {
		C.free(p)
	}
}

// SessionStoreFor returns the libsignal SessionStore wired to h. h must
// hold a [store.SessionStore].
func SessionStoreFor(h *StoreHandle) C.SignalSessionStore {
	_ = h.value.(store.SessionStore) // compile-time-ish assertion for the bug-finder
	return C.signalgo_session_store()
}

// IdentityKeyStoreFor returns the libsignal IdentityKeyStore wired to h.
// h must hold a [store.IdentityStore].
func IdentityKeyStoreFor(h *StoreHandle) C.SignalIdentityKeyStore {
	_ = h.value.(store.IdentityStore)
	return C.signalgo_identity_key_store()
}

// PreKeyStoreFor returns the libsignal PreKeyStore wired to h.
// h must hold a [store.PreKeyStore].
func PreKeyStoreFor(h *StoreHandle) C.SignalPreKeyStore {
	_ = h.value.(store.PreKeyStore)
	return C.signalgo_pre_key_store()
}

// SignedPreKeyStoreFor returns the libsignal SignedPreKeyStore wired to h.
// h must hold a [store.SignedPreKeyStore].
func SignedPreKeyStoreFor(h *StoreHandle) C.SignalSignedPreKeyStore {
	_ = h.value.(store.SignedPreKeyStore)
	return C.signalgo_signed_pre_key_store()
}

// KyberPreKeyStoreFor returns the libsignal KyberPreKeyStore wired to h.
// h must hold a [store.KyberPreKeyStore].
func KyberPreKeyStoreFor(h *StoreHandle) C.SignalKyberPreKeyStore {
	_ = h.value.(store.KyberPreKeyStore)
	return C.signalgo_kyber_pre_key_store()
}

// SenderKeyStoreFor returns the libsignal SenderKeyStore wired to h.
// h must hold a [store.SenderKeyStore].
func SenderKeyStoreFor(h *StoreHandle) C.SignalSenderKeyStore {
	_ = h.value.(store.SenderKeyStore)
	return C.signalgo_sender_key_store()
}

// SessionStoreStruct returns the FFI session store wired to h's ctx.
func (h *StoreHandle) SessionStoreStruct() C.SignalConstPointerFfiSessionStoreStruct {
	return C.SignalConstPointerFfiSessionStoreStruct{raw: h.cSession}
}

// IdentityKeyStoreStruct returns the FFI identity store wired to h's ctx.
func (h *StoreHandle) IdentityKeyStoreStruct() C.SignalConstPointerFfiIdentityKeyStoreStruct {
	return C.SignalConstPointerFfiIdentityKeyStoreStruct{raw: h.cIdentity}
}

// PreKeyStoreStruct returns the FFI prekey store wired to h's ctx.
func (h *StoreHandle) PreKeyStoreStruct() C.SignalConstPointerFfiPreKeyStoreStruct {
	return C.SignalConstPointerFfiPreKeyStoreStruct{raw: h.cPreKey}
}

// SignedPreKeyStoreStruct returns the FFI signed-prekey store wired to h's ctx.
func (h *StoreHandle) SignedPreKeyStoreStruct() C.SignalConstPointerFfiSignedPreKeyStoreStruct {
	return C.SignalConstPointerFfiSignedPreKeyStoreStruct{raw: h.cSigned}
}

// KyberPreKeyStoreStruct returns the FFI Kyber-prekey store wired to h's ctx.
func (h *StoreHandle) KyberPreKeyStoreStruct() C.SignalConstPointerFfiKyberPreKeyStoreStruct {
	return C.SignalConstPointerFfiKyberPreKeyStoreStruct{raw: h.cKyber}
}
