package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"
)

const profileKeyCommitmentLen = C.SignalPROFILE_KEY_COMMITMENT_LEN

// TestingProfileKeyPresentationRoundTrip builds a valid profile-key
// credential presentation for unit tests using deterministic server params.
func TestingProfileKeyPresentationRoundTrip(
	aci string,
	profileKey []byte,
	masterKey []byte,
) ([]byte, error) {
	var serverRandomness [ZKRandomnessLen]byte
	for i := range serverRandomness {
		serverRandomness[i] = byte(i + 1)
	}
	var serverSecret C.SignalMutPointerServerSecretParams
	if err := checkError(C.signal_server_secret_params_generate_deterministic(&serverSecret, cRandomnessIn(&serverRandomness))); err != nil {
		return nil, err
	}
	defer func() { _ = checkError(C.signal_server_secret_params_destroy(serverSecret)) }()

	serverSecretConst := C.SignalConstPointerServerSecretParams(serverSecret)
	var serverPublic C.SignalMutPointerServerPublicParams
	if err := checkError(C.signal_server_secret_params_get_public_params(&serverPublic, serverSecretConst)); err != nil {
		return nil, err
	}
	defer func() { _ = checkError(C.signal_server_public_params_destroy(serverPublic)) }()

	serverParams := &ServerPublicParams{raw: serverPublic}
	secretParams, err := GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	user, err := ParseServiceIDString(aci)
	if err != nil {
		return nil, err
	}

	var reqRandomness [ZKRandomnessLen]byte
	for i := range reqRandomness {
		reqRandomness[i] = byte(i + 4)
	}
	reqCtx, err := CreateProfileKeyCredentialRequestContext(serverParams, user, profileKey, reqRandomness)
	if err != nil {
		return nil, err
	}
	request, err := ProfileKeyCredentialRequestFromContext(reqCtx)
	if err != nil {
		return nil, err
	}

	var commitment [profileKeyCommitmentLen]byte
	if err := checkError(C.signal_profile_key_get_commitment(
		cProfileKeyCommitmentOut(&commitment),
		cProfileKeyIn(profileKey),
		cServiceID(user),
	)); err != nil {
		return nil, err
	}

	var issueRandomness [ZKRandomnessLen]byte
	for i := range issueRandomness {
		issueRandomness[i] = byte(i + 5)
	}
	var response [ExpiringProfileKeyCredentialResponseLen]byte
	expiration := uint64(17 * 24 * 60 * 60)
	currentTime := expiration - 2*24*60*60
	if err := checkError(C.signal_server_secret_params_issue_expiring_profile_key_credential_deterministic(
		cExpiringProfileKeyCredentialResponseOut(response[:]),
		serverSecretConst,
		cRandomnessIn(&issueRandomness),
		cProfileKeyCredentialRequestIn(&request),
		cServiceID(user),
		cProfileKeyCommitmentIn(&commitment),
		C.uint64_t(expiration),
	)); err != nil {
		return nil, err
	}

	credential, err := ReceiveExpiringProfileKeyCredential(serverParams, reqCtx, response[:], time.Unix(int64(currentTime), 0))
	if err != nil {
		return nil, fmt.Errorf("receive credential: %w", err)
	}

	var presRandomness [ZKRandomnessLen]byte
	for i := range presRandomness {
		presRandomness[i] = byte(i + 6)
	}
	return CreateExpiringProfileKeyCredentialPresentation(serverParams, secretParams, credential, presRandomness)
}

func cProfileKeyCredentialRequestIn(b *[ProfileKeyCredentialRequestLen]byte) *[C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_CREDENTIAL_REQUEST_LEN]C.uchar)(unsafe.Pointer(b))
}

func cExpiringProfileKeyCredentialResponseOut(b []byte) *[C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_RESPONSE_LEN]C.uchar {
	return (*[C.SignalEXPIRING_PROFILE_KEY_CREDENTIAL_RESPONSE_LEN]C.uchar)(unsafe.Pointer(&b[0]))
}

func cProfileKeyCommitmentOut(b *[profileKeyCommitmentLen]byte) *[C.SignalPROFILE_KEY_COMMITMENT_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_COMMITMENT_LEN]C.uchar)(unsafe.Pointer(b))
}

func cProfileKeyCommitmentIn(b *[profileKeyCommitmentLen]byte) *[C.SignalPROFILE_KEY_COMMITMENT_LEN]C.uchar {
	return (*[C.SignalPROFILE_KEY_COMMITMENT_LEN]C.uchar)(unsafe.Pointer(b))
}
