package libsignal

/*
#include <stdlib.h>
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// UsernameHashLen is the byte length of a Signal username hash.
const UsernameHashLen = 32

// UsernameCandidate is one `nickname.discriminator` string produced by
// [UsernameCandidates]. The caller picks one and reserves its hash with
// the server. The string shape is `<nickname>.<discriminator>` and is
// safe to display and to hash with [UsernameHash].
type UsernameCandidate string

// UsernameCandidates generates up to a small list of candidate usernames
// derived from nickname. min and max bound the total username length
// (nickname + '.' + discriminator) in display characters. libsignal
// enforces the standard range (3..=32) internally; pass 0 to accept the
// default.
//
// This is a local operation; it does not talk to the server. The caller
// is expected to display these to the user (or, for a bot, to pick one
// deterministically) and then reserve the chosen hash via the
// authenticated chat connection.
func UsernameCandidates(nickname string, minLen, maxLen uint32) ([]UsernameCandidate, error) {
	if nickname == "" {
		return nil, errors.New("libsignal.UsernameCandidates: empty nickname")
	}
	cNick := C.CString(nickname)
	defer C.free(unsafe.Pointer(cNick))

	var raw C.SignalBytestringArray
	if err := checkError(C.signal_username_candidates_from(
		&raw,
		(*C.int8_t)(unsafe.Pointer(cNick)),
		C.uint32_t(minLen),
		C.uint32_t(maxLen),
	)); err != nil {
		return nil, err
	}
	defer C.signal_free_bytestring_array(raw)

	return decodeBytestringArray(raw), nil
}

// UsernameHash returns the 32-byte server hash of username. username is
// the full `nickname.discriminator` string.
func UsernameHash(username string) ([UsernameHashLen]byte, error) {
	var out [UsernameHashLen]byte
	if username == "" {
		return out, errors.New("libsignal.UsernameHash: empty username")
	}
	cUser := C.CString(username)
	defer C.free(unsafe.Pointer(cUser))

	var raw [UsernameHashLen]C.uint8_t
	if err := checkError(C.signal_username_hash(
		(*C.SignalType_FixedArray32_uint8_t)(unsafe.Pointer(&raw)),
		(*C.int8_t)(unsafe.Pointer(cUser)),
	)); err != nil {
		return out, err
	}
	for i := range out {
		out[i] = byte(raw[i])
	}
	return out, nil
}

// UsernameHashFromParts hashes a username assembled from its nickname
// and discriminator parts. Useful when the two are stored separately.
func UsernameHashFromParts(nickname, discriminator string, minLen, maxLen uint32) ([UsernameHashLen]byte, error) {
	var out [UsernameHashLen]byte
	if nickname == "" || discriminator == "" {
		return out, errors.New("libsignal.UsernameHashFromParts: empty nickname or discriminator")
	}
	cNick := C.CString(nickname)
	defer C.free(unsafe.Pointer(cNick))
	cDisc := C.CString(discriminator)
	defer C.free(unsafe.Pointer(cDisc))

	var raw [UsernameHashLen]C.uint8_t
	if err := checkError(C.signal_username_hash_from_parts(
		(*C.SignalType_FixedArray32_uint8_t)(unsafe.Pointer(&raw)),
		(*C.int8_t)(unsafe.Pointer(cNick)),
		(*C.int8_t)(unsafe.Pointer(cDisc)),
		C.uint32_t(minLen),
		C.uint32_t(maxLen),
	)); err != nil {
		return out, err
	}
	for i := range out {
		out[i] = byte(raw[i])
	}
	return out, nil
}

// UsernameProof generates a zero-knowledge proof of ownership for
// username using the supplied 32-byte randomness. The server verifies
// the proof against the username hash on the account.
func UsernameProof(username string, randomness [32]byte) ([]byte, error) {
	if username == "" {
		return nil, errors.New("libsignal.UsernameProof: empty username")
	}
	cUser := C.CString(username)
	defer C.free(unsafe.Pointer(cUser))

	var raw C.SignalOwnedBuffer
	err := checkError(C.signal_username_proof(
		&raw,
		(*C.int8_t)(unsafe.Pointer(cUser)),
		(*[32]C.uint8_t)(unsafe.Pointer(&randomness[0])),
	))
	keepAlive(randomness[:])
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(raw), nil
}

// VerifyUsernameProof verifies that proof was generated from a username
// whose hash matches hash.
func VerifyUsernameProof(proof []byte, hash [UsernameHashLen]byte) error {
	if len(proof) == 0 {
		return errors.New("libsignal.VerifyUsernameProof: empty proof")
	}
	hashBuf := make([]byte, UsernameHashLen)
	copy(hashBuf, hash[:])
	err := checkError(C.signal_username_verify(borrowed(proof), borrowed(hashBuf)))
	keepAlive(proof)
	keepAlive(hashBuf)
	return err
}

// UsernameLinkCreate encrypts username with entropy and returns the
// encrypted ciphertext that the server will store under a UUID handle.
// The caller publishes (handle, entropy) as the link; anyone with the
// link can call [UsernameLinkDecrypt] to recover the plaintext
// username after fetching the ciphertext from the server.
//
// If entropy is nil, libsignal generates fresh random entropy; pass
// non-nil entropy to rotate an existing link in place.
func UsernameLinkCreate(username string, entropy []byte) ([]byte, error) {
	if username == "" {
		return nil, errors.New("libsignal.UsernameLinkCreate: empty username")
	}
	cUser := C.CString(username)
	defer C.free(unsafe.Pointer(cUser))

	var raw C.SignalOwnedBuffer
	err := checkError(C.signal_username_link_create(
		&raw,
		(*C.int8_t)(unsafe.Pointer(cUser)),
		borrowed(entropy),
	))
	keepAlive(entropy)
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(raw), nil
}

// UsernameLinkDecrypt reverses [UsernameLinkCreate]: given the entropy
// from the link and the encrypted username payload fetched from the
// server, it returns the plaintext `nickname.discriminator` string.
func UsernameLinkDecrypt(entropy, encryptedUsername []byte) (string, error) {
	if len(entropy) == 0 || len(encryptedUsername) == 0 {
		return "", errors.New("libsignal.UsernameLinkDecrypt: empty entropy or ciphertext")
	}
	var cstr C.SignalCStringPtr
	err := checkError(C.signal_username_link_decrypt_username(
		&cstr,
		borrowed(entropy),
		borrowed(encryptedUsername),
	))
	keepAlive(entropy)
	keepAlive(encryptedUsername)
	if err != nil {
		return "", err
	}
	defer C.signal_free_string((*C.int8_t)(unsafe.Pointer(cstr)))
	return C.GoString((*C.char)(unsafe.Pointer(cstr))), nil
}

// decodeBytestringArray splits a SignalBytestringArray into Go strings.
// The array is a single flat byte buffer with a parallel per-entry
// length slice. The caller frees the array after this returns.
func decodeBytestringArray(arr C.SignalBytestringArray) []UsernameCandidate {
	n := int(arr.lengths.length)
	if n == 0 || arr.bytes.base == nil {
		return nil
	}
	lens := unsafe.Slice((*C.size_t)(unsafe.Pointer(arr.lengths.base)), n)
	flat := C.GoBytes(unsafe.Pointer(arr.bytes.base), C.int(arr.bytes.length))
	out := make([]UsernameCandidate, 0, n)
	off := 0
	for _, l := range lens {
		end := off + int(l)
		if end > len(flat) {
			// Should not happen; libsignal's lens array lies inside bytes.
			return out
		}
		out = append(out, UsernameCandidate(flat[off:end]))
		off = end
	}
	return out
}

// usernameString is a defensive helper used by tests; keeps the error
// message consistent across call sites.
func usernameErr(fn string, err error) error {
	return fmt.Errorf("libsignal.%s: %w", fn, err)
}
