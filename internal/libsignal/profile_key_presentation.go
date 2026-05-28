package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

const (
	// ProfileKeyCredentialRequestLen is the serialized credential request size.
	ProfileKeyCredentialRequestLen = C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_LEN
	// ProfileKeyCredentialRequestContextLen is the request context size.
	ProfileKeyCredentialRequestContextLen = C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_CONTEXT_LEN
	// ExpiringProfileKeyCredentialLen is a received expiring profile key credential.
	ExpiringProfileKeyCredentialLen = C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_LEN
	// ExpiringProfileKeyCredentialResponseLen is the server credential response.
	ExpiringProfileKeyCredentialResponseLen = C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_RESPONSE_LEN
	// ProfileKeyCiphertextLen is the encrypted profile key in a presentation.
	ProfileKeyCiphertextLen = C.SignalPROFILE_KEY_CIPHERTEXT_LEN
)

// CreateProfileKeyCredentialRequestContext builds a zkgroup request context
// for fetching an expiring profile key credential from the chat service.
func CreateProfileKeyCredentialRequestContext(
	serverParams *ServerPublicParams,
	user ServiceIDFixedWidth,
	profileKey []byte,
	randomness [ZKRandomnessLen]byte,
) ([ProfileKeyCredentialRequestContextLen]byte, error) {
	var out [ProfileKeyCredentialRequestContextLen]byte
	if serverParams == nil {
		return out, errors.New("libsignal.CreateProfileKeyCredentialRequestContext: nil server params")
	}
	if len(profileKey) != ProfileKeyLen {
		return out, fmt.Errorf("libsignal.CreateProfileKeyCredentialRequestContext: profile key length %d, want %d", len(profileKey), ProfileKeyLen)
	}
	if err := checkError(C.signal_server_public_params_create_profile_key_credential_request_context_deterministic(
		cProfileKeyCredentialRequestContextOut(&out),
		serverParams.constPtr(),
		cRandomnessIn(&randomness),
		cServiceID(user),
		cProfileKeyIn(profileKey),
	)); err != nil {
		return out, err
	}
	runtime.KeepAlive(serverParams)
	keepAlive(profileKey)
	return out, nil
}

// ProfileKeyCredentialRequestFromContext extracts the request bytes sent to
// the chat service when fetching a profile key credential.
func ProfileKeyCredentialRequestFromContext(context [ProfileKeyCredentialRequestContextLen]byte) ([ProfileKeyCredentialRequestLen]byte, error) {
	var out [ProfileKeyCredentialRequestLen]byte
	if err := checkError(C.signal_profile_key_credential_request_context_get_request(
		cProfileKeyCredentialRequestOut(&out),
		cProfileKeyCredentialRequestContextIn(&context),
	)); err != nil {
		return out, err
	}
	return out, nil
}

// ReceiveExpiringProfileKeyCredential converts a server credential response
// into a stored expiring profile key credential.
func ReceiveExpiringProfileKeyCredential(
	serverParams *ServerPublicParams,
	context [ProfileKeyCredentialRequestContextLen]byte,
	response []byte,
	now time.Time,
) ([ExpiringProfileKeyCredentialLen]byte, error) {
	var out [ExpiringProfileKeyCredentialLen]byte
	if serverParams == nil {
		return out, errors.New("libsignal.ReceiveExpiringProfileKeyCredential: nil server params")
	}
	if len(response) != ExpiringProfileKeyCredentialResponseLen {
		return out, fmt.Errorf("libsignal.ReceiveExpiringProfileKeyCredential: response length %d, want %d", len(response), ExpiringProfileKeyCredentialResponseLen)
	}
	if err := checkError(C.signal_server_public_params_receive_expiring_profile_key_credential(
		cExpiringProfileKeyCredentialOut(&out),
		serverParams.constPtr(),
		cProfileKeyCredentialRequestContextIn(&context),
		cExpiringProfileKeyCredentialResponseIn(response),
		C.uint64_t(now.Unix()),
	)); err != nil {
		return out, err
	}
	runtime.KeepAlive(serverParams)
	keepAlive(response)
	return out, nil
}

// CreateExpiringProfileKeyCredentialPresentation builds a profile-key
// credential presentation for Groups v2 add-member actions.
func CreateExpiringProfileKeyCredentialPresentation(
	serverParams *ServerPublicParams,
	groupSecretParams [GroupSecretParamsLen]byte,
	credential [ExpiringProfileKeyCredentialLen]byte,
	randomness [ZKRandomnessLen]byte,
) ([]byte, error) {
	if serverParams == nil {
		return nil, errors.New("libsignal.CreateExpiringProfileKeyCredentialPresentation: nil server params")
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_server_public_params_create_expiring_profile_key_credential_presentation_deterministic(
		&buf,
		serverParams.constPtr(),
		cRandomnessIn(&randomness),
		cGroupSecretParamsIn(&groupSecretParams),
		cExpiringProfileKeyCredentialIn(&credential),
	)); err != nil {
		return nil, err
	}
	runtime.KeepAlive(serverParams)
	return goBytesFromOwnedBuffer(buf), nil
}

// ProfileKeyPresentationProfileKeyCiphertext extracts the encrypted profile key
// from a profile-key credential presentation.
func ProfileKeyPresentationProfileKeyCiphertext(presentation []byte) ([ProfileKeyCiphertextLen]byte, error) {
	var out [ProfileKeyCiphertextLen]byte
	if len(presentation) == 0 {
		return out, errors.New("libsignal.ProfileKeyPresentationProfileKeyCiphertext: empty presentation")
	}
	if err := checkError(C.signal_profile_key_credential_presentation_check_valid_contents(borrowed(presentation))); err != nil {
		return out, err
	}
	if err := checkError(C.signal_profile_key_credential_presentation_get_profile_key_ciphertext(
		cProfileKeyCiphertextIn(&out),
		borrowed(presentation),
	)); err != nil {
		return out, err
	}
	keepAlive(presentation)
	return out, nil
}

// ProfileKeyPresentationUUIDCiphertext extracts the encrypted service id from
// a profile-key credential presentation.
func ProfileKeyPresentationUUIDCiphertext(presentation []byte) ([UUIDCiphertextLen]byte, error) {
	var out [UUIDCiphertextLen]byte
	if len(presentation) == 0 {
		return out, errors.New("libsignal.ProfileKeyPresentationUUIDCiphertext: empty presentation")
	}
	if err := checkError(C.signal_profile_key_credential_presentation_check_valid_contents(borrowed(presentation))); err != nil {
		return out, err
	}
	if err := checkError(C.signal_profile_key_credential_presentation_get_uuid_ciphertext(
		cUUIDCiphertextOut(&out),
		borrowed(presentation),
	)); err != nil {
		return out, err
	}
	keepAlive(presentation)
	return out, nil
}

// GroupSecretParamsDecryptProfileKey decrypts a profile key ciphertext for a
// member service id using group secret params.
func GroupSecretParamsDecryptProfileKey(
	secretParams [GroupSecretParamsLen]byte,
	ciphertext [ProfileKeyCiphertextLen]byte,
	user ServiceIDFixedWidth,
) ([ProfileKeyLen]byte, error) {
	var out [ProfileKeyLen]byte
	if err := checkError(C.signal_group_secret_params_decrypt_profile_key(
		cProfileKeyOut(&out),
		cGroupSecretParamsIn(&secretParams),
		cProfileKeyCiphertextIn(&ciphertext),
		cServiceID(user),
	)); err != nil {
		return out, err
	}
	return out, nil
}

func cProfileKeyIn(b []byte) *[C.SignalPROFILE_KEY_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_LEN]C.uchar)(unsafe.Pointer(&b[0]))
}

func cProfileKeyOut(b *[ProfileKeyLen]byte) *[C.SignalPROFILE_KEY_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_LEN]C.uchar)(unsafe.Pointer(b))
}

func cProfileKeyCredentialRequestContextIn(b *[ProfileKeyCredentialRequestContextLen]byte) *[C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_CONTEXT_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_CONTEXT_LEN]C.uchar)(unsafe.Pointer(b))
}

func cProfileKeyCredentialRequestContextOut(b *[ProfileKeyCredentialRequestContextLen]byte) *[C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_CONTEXT_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_CONTEXT_LEN]C.uchar)(unsafe.Pointer(b))
}

func cProfileKeyCredentialRequestOut(b *[ProfileKeyCredentialRequestLen]byte) *[C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_LEN]C.uchar)(unsafe.Pointer(b))
}

func cExpiringProfileKeyCredentialOut(b *[ExpiringProfileKeyCredentialLen]byte) *[C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_LEN]C.uchar {
	return (*[C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_LEN]C.uchar)(unsafe.Pointer(b))
}

func cExpiringProfileKeyCredentialIn(b *[ExpiringProfileKeyCredentialLen]byte) *[C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_LEN]C.uchar {
	return (*[C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_LEN]C.uchar)(unsafe.Pointer(b))
}

func cExpiringProfileKeyCredentialResponseIn(b []byte) *[C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_RESPONSE_LEN]C.uchar {
	return (*[C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_RESPONSE_LEN]C.uchar)(unsafe.Pointer(&b[0]))
}

func cProfileKeyCiphertextIn(b *[ProfileKeyCiphertextLen]byte) *[C.SignalPROFILE_KEY_CIPHERTEXT_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_CIPHERTEXT_LEN]C.uchar)(unsafe.Pointer(b))
}
