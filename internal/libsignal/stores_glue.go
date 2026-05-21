package libsignal

/*
#include "signal_ffi.h"

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
	"runtime/cgo"

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
}

// NewStoreHandle registers any store implementation through a
// [runtime/cgo.Handle]. The handle can then be used to materialise the
// libsignal callback structs.
func NewStoreHandle(s any) *StoreHandle {
	return &StoreHandle{value: s, h: cgo.NewHandle(s)}
}

// Ctx returns the void* libsignal expects in the callback ctx slot.
func (h *StoreHandle) Ctx() C.uintptr_t { return C.uintptr_t(h.h) }

// Release frees the cgo handle. Idempotent.
func (h *StoreHandle) Release() {
	if h == nil || h.h == 0 {
		return
	}
	h.h.Delete()
	h.h = 0
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
