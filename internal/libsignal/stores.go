package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"unsafe"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// This file bridges Signal's libsignal-FFI callback structs into Go
// implementations of the [store] package's sub-interfaces.
//
// Each callback comes in two layers:
//
//  1. A //export'd C-callable function (signalgo_*) with the exact
//     signature libsignal's typedef expects. It does the C↔Go conversion
//     (serializing incoming libsignal records to blobs, deserializing
//     outgoing blobs back into libsignal records) and delegates the
//     business logic to:
//  2. A pure-Go-typed implementation in stores_impl.go that takes Go
//     arguments and returns Go errors. Tests exercise this layer
//     directly with no cgo.
//
// Return-code convention at the FFI layer:
//
//	0  = success
//	1  = "not found" (Load* only; other callbacks treat this as error)
//	-1 = error
//
// Out-parameters are written iff we return 0.

// loadReturnCode maps a Go (data, err) to libsignal's int return.
func loadReturnCode(err error) C.int {
	if err == nil {
		return 0
	}
	if errors.Is(err, store.ErrRecordNotFound) {
		return 1
	}
	return -1
}

func okOrError(err error) C.int {
	if err == nil {
		return 0
	}
	return -1
}

// addressFromC reads a libsignal ProtocolAddress (passed by value as a
// mutable pointer) into a Go [store.Address]. The libsignal pointer
// remains owned by libsignal; we only read from it.
func addressFromC(p C.SignalMutPointerProtocolAddress) (store.Address, error) {
	cp := C.SignalConstPointerProtocolAddress{raw: p.raw}
	var cname *C.char
	if err := checkError(C.signal_address_get_name(&cname, cp)); err != nil {
		return store.Address{}, err
	}
	name := C.GoString(cname)
	C.signal_free_string(cname)
	var dev C.uint32_t
	if err := checkError(C.signal_address_get_device_id(&dev, cp)); err != nil {
		return store.Address{}, err
	}
	return store.Address{ServiceID: name, DeviceID: uint32(dev)}, nil
}

// uuidFromC reads a SignalUuid (16-byte array) and returns its canonical
// hex-dash form.
func uuidFromC(u C.SignalUuid) string {
	var b [16]byte
	for i := 0; i < 16; i++ {
		b[i] = byte(u.bytes[i])
	}
	return formatUUID(b)
}

// publicKeyToBytes serializes a libsignal public key pointer to its
// 33-byte tagged encoding.
func publicKeyToBytes(p C.SignalMutPointerPublicKey) ([]byte, error) {
	cp := C.SignalConstPointerPublicKey{raw: p.raw}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_publickey_serialize(&buf, cp)); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// sessionRecordToBytes serializes a libsignal session record pointer.
func sessionRecordToBytes(p C.SignalMutPointerSessionRecord) ([]byte, error) {
	cp := C.SignalConstPointerSessionRecord{raw: p.raw}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_session_record_serialize(&buf, cp)); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// preKeyRecordToBytes serializes a libsignal prekey record pointer.
func preKeyRecordToBytes(p C.SignalMutPointerPreKeyRecord) ([]byte, error) {
	cp := C.SignalConstPointerPreKeyRecord{raw: p.raw}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_pre_key_record_serialize(&buf, cp)); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// signedPreKeyRecordToBytes serializes a libsignal signed prekey record.
func signedPreKeyRecordToBytes(p C.SignalMutPointerSignedPreKeyRecord) ([]byte, error) {
	cp := C.SignalConstPointerSignedPreKeyRecord{raw: p.raw}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_signed_pre_key_record_serialize(&buf, cp)); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// kyberPreKeyRecordToBytes serializes a libsignal Kyber prekey record.
func kyberPreKeyRecordToBytes(p C.SignalMutPointerKyberPreKeyRecord) ([]byte, error) {
	cp := C.SignalConstPointerKyberPreKeyRecord{raw: p.raw}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_kyber_pre_key_record_serialize(&buf, cp)); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// senderKeyRecordToBytes serializes a libsignal sender-key record.
func senderKeyRecordToBytes(p C.SignalMutPointerSenderKeyRecord) ([]byte, error) {
	cp := C.SignalConstPointerSenderKeyRecord{raw: p.raw}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_sender_key_record_serialize(&buf, cp)); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// handleAs recovers the Go store of type S from ctx. Returns false if ctx
// is invalid or the stored value does not implement S.
func handleAs[S any](ctx unsafe.Pointer) (S, bool) {
	var zero S
	if ctx == nil {
		return zero, false
	}
	v := cgo.Handle(uintptr(ctx)).Value()
	s, ok := v.(S)
	return s, ok
}

// --- //export shells ---

//export signalgo_load_session
func signalgo_load_session(ctx unsafe.Pointer, out *C.SignalMutPointerSessionRecord, address C.SignalMutPointerProtocolAddress) C.int {
	s, ok := handleAs[store.SessionStore](ctx)
	if !ok {
		return -1
	}
	addr, err := addressFromC(address)
	if err != nil {
		return -1
	}
	blob, err := loadSessionImpl(s, addr)
	if err != nil {
		return loadReturnCode(err)
	}
	rec, err := DeserializeSessionRecord(blob)
	if err != nil {
		return -1
	}
	*out = rec.rawMut()
	rec.raw.raw = nil
	return 0
}

//export signalgo_store_session
func signalgo_store_session(ctx unsafe.Pointer, address C.SignalMutPointerProtocolAddress, record C.SignalMutPointerSessionRecord) C.int {
	s, ok := handleAs[store.SessionStore](ctx)
	if !ok {
		return -1
	}
	addr, err := addressFromC(address)
	if err != nil {
		return -1
	}
	blob, err := sessionRecordToBytes(record)
	if err != nil {
		return -1
	}
	return okOrError(storeSessionImpl(s, addr, blob))
}

//export signalgo_get_local_identity_key_pair
func signalgo_get_local_identity_key_pair(ctx unsafe.Pointer, out *C.SignalPairOfMutPointerPrivateKeyMutPointerPublicKey) C.int {
	s, ok := handleAs[store.IdentityStore](ctx)
	if !ok {
		return -1
	}
	pubBytes, privBytes, err := getLocalIdentityKeyPairImpl(s)
	if err != nil {
		return loadReturnCode(err)
	}
	priv, err := DeserializePrivateKey(privBytes)
	if err != nil {
		return -1
	}
	pub, err := DeserializePublicKey(pubBytes)
	if err != nil {
		return -1
	}
	out.first = priv.raw
	out.second = pub.raw
	priv.raw.raw = nil
	pub.raw.raw = nil
	return 0
}

//export signalgo_get_local_registration_id
func signalgo_get_local_registration_id(ctx unsafe.Pointer, out *C.uint32_t) C.int {
	s, ok := handleAs[store.IdentityStore](ctx)
	if !ok {
		return -1
	}
	id, err := getLocalRegistrationIDImpl(s)
	if err != nil {
		return -1
	}
	*out = C.uint32_t(id)
	return 0
}

//export signalgo_get_identity_key
func signalgo_get_identity_key(ctx unsafe.Pointer, out *C.SignalMutPointerPublicKey, address C.SignalMutPointerProtocolAddress) C.int {
	s, ok := handleAs[store.IdentityStore](ctx)
	if !ok {
		return -1
	}
	addr, err := addressFromC(address)
	if err != nil {
		return -1
	}
	pubBytes, err := getIdentityKeyImpl(s, addr)
	if err != nil {
		return loadReturnCode(err)
	}
	pub, err := DeserializePublicKey(pubBytes)
	if err != nil {
		return -1
	}
	*out = pub.raw
	pub.raw.raw = nil
	return 0
}

//export signalgo_save_identity_key
func signalgo_save_identity_key(ctx unsafe.Pointer, out *C.uint8_t, address C.SignalMutPointerProtocolAddress, publicKey C.SignalMutPointerPublicKey) C.int {
	s, ok := handleAs[store.IdentityStore](ctx)
	if !ok {
		return -1
	}
	addr, err := addressFromC(address)
	if err != nil {
		return -1
	}
	pubBytes, err := publicKeyToBytes(publicKey)
	if err != nil {
		return -1
	}
	res, err := saveIdentityKeyImpl(s, addr, pubBytes)
	if err != nil {
		return -1
	}
	*out = C.uint8_t(res)
	return 0
}

//export signalgo_is_trusted_identity
func signalgo_is_trusted_identity(ctx unsafe.Pointer, out *C.bool, address C.SignalMutPointerProtocolAddress, publicKey C.SignalMutPointerPublicKey, direction C.uint32_t) C.int {
	s, ok := handleAs[store.IdentityStore](ctx)
	if !ok {
		return -1
	}
	addr, err := addressFromC(address)
	if err != nil {
		return -1
	}
	pubBytes, err := publicKeyToBytes(publicKey)
	if err != nil {
		return -1
	}
	trusted, err := isTrustedIdentityImpl(s, addr, pubBytes, store.Direction(direction))
	if err != nil {
		return -1
	}
	*out = C.bool(trusted)
	return 0
}

//export signalgo_load_pre_key
func signalgo_load_pre_key(ctx unsafe.Pointer, out *C.SignalMutPointerPreKeyRecord, id C.uint32_t) C.int {
	s, ok := handleAs[store.PreKeyStore](ctx)
	if !ok {
		return -1
	}
	blob, err := loadPreKeyImpl(s, uint32(id))
	if err != nil {
		return loadReturnCode(err)
	}
	rec, err := DeserializePreKeyRecord(blob)
	if err != nil {
		return -1
	}
	*out = rec.rawMut()
	rec.raw.raw = nil
	return 0
}

//export signalgo_store_pre_key
func signalgo_store_pre_key(ctx unsafe.Pointer, id C.uint32_t, record C.SignalMutPointerPreKeyRecord) C.int {
	s, ok := handleAs[store.PreKeyStore](ctx)
	if !ok {
		return -1
	}
	blob, err := preKeyRecordToBytes(record)
	if err != nil {
		return -1
	}
	return okOrError(storePreKeyImpl(s, uint32(id), blob))
}

//export signalgo_remove_pre_key
func signalgo_remove_pre_key(ctx unsafe.Pointer, id C.uint32_t) C.int {
	s, ok := handleAs[store.PreKeyStore](ctx)
	if !ok {
		return -1
	}
	return okOrError(removePreKeyImpl(s, uint32(id)))
}

//export signalgo_load_signed_pre_key
func signalgo_load_signed_pre_key(ctx unsafe.Pointer, out *C.SignalMutPointerSignedPreKeyRecord, id C.uint32_t) C.int {
	s, ok := handleAs[store.SignedPreKeyStore](ctx)
	if !ok {
		return -1
	}
	blob, err := loadSignedPreKeyImpl(s, uint32(id))
	if err != nil {
		return loadReturnCode(err)
	}
	rec, err := DeserializeSignedPreKeyRecord(blob)
	if err != nil {
		return -1
	}
	*out = rec.rawMut()
	rec.raw.raw = nil
	return 0
}

//export signalgo_store_signed_pre_key
func signalgo_store_signed_pre_key(ctx unsafe.Pointer, id C.uint32_t, record C.SignalMutPointerSignedPreKeyRecord) C.int {
	s, ok := handleAs[store.SignedPreKeyStore](ctx)
	if !ok {
		return -1
	}
	blob, err := signedPreKeyRecordToBytes(record)
	if err != nil {
		return -1
	}
	return okOrError(storeSignedPreKeyImpl(s, uint32(id), blob))
}

//export signalgo_load_kyber_pre_key
func signalgo_load_kyber_pre_key(ctx unsafe.Pointer, out *C.SignalMutPointerKyberPreKeyRecord, id C.uint32_t) C.int {
	s, ok := handleAs[store.KyberPreKeyStore](ctx)
	if !ok {
		return -1
	}
	blob, err := loadKyberPreKeyImpl(s, uint32(id))
	if err != nil {
		return loadReturnCode(err)
	}
	rec, err := DeserializeKyberPreKeyRecord(blob)
	if err != nil {
		return -1
	}
	*out = rec.rawMut()
	rec.raw.raw = nil
	return 0
}

//export signalgo_store_kyber_pre_key
func signalgo_store_kyber_pre_key(ctx unsafe.Pointer, id C.uint32_t, record C.SignalMutPointerKyberPreKeyRecord) C.int {
	s, ok := handleAs[store.KyberPreKeyStore](ctx)
	if !ok {
		return -1
	}
	blob, err := kyberPreKeyRecordToBytes(record)
	if err != nil {
		return -1
	}
	return okOrError(storeKyberPreKeyImpl(s, uint32(id), blob))
}

//export signalgo_mark_kyber_pre_key_used
func signalgo_mark_kyber_pre_key_used(ctx unsafe.Pointer, id C.uint32_t, _ecPreKeyID C.uint32_t, _baseKey C.SignalMutPointerPublicKey) C.int {
	s, ok := handleAs[store.KyberPreKeyStore](ctx)
	if !ok {
		return -1
	}
	return okOrError(markKyberPreKeyUsedImpl(s, uint32(id)))
}

//export signalgo_load_sender_key
func signalgo_load_sender_key(ctx unsafe.Pointer, out *C.SignalMutPointerSenderKeyRecord, sender C.SignalMutPointerProtocolAddress, distID C.SignalUuid) C.int {
	s, ok := handleAs[store.SenderKeyStore](ctx)
	if !ok {
		return -1
	}
	addr, err := addressFromC(sender)
	if err != nil {
		return -1
	}
	blob, err := loadSenderKeyImpl(s, addr, uuidFromC(distID))
	if err != nil {
		return loadReturnCode(err)
	}
	rec, err := DeserializeSenderKeyRecord(blob)
	if err != nil {
		return -1
	}
	*out = rec.rawMut()
	rec.raw.raw = nil
	return 0
}

//export signalgo_store_sender_key
func signalgo_store_sender_key(ctx unsafe.Pointer, sender C.SignalMutPointerProtocolAddress, distID C.SignalUuid, record C.SignalMutPointerSenderKeyRecord) C.int {
	s, ok := handleAs[store.SenderKeyStore](ctx)
	if !ok {
		return -1
	}
	addr, err := addressFromC(sender)
	if err != nil {
		return -1
	}
	blob, err := senderKeyRecordToBytes(record)
	if err != nil {
		return -1
	}
	return okOrError(storeSenderKeyImpl(s, addr, uuidFromC(distID), blob))
}

//export signalgo_destroy_store_handle
func signalgo_destroy_store_handle(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}
	cgo.Handle(uintptr(ctx)).Delete()
}
