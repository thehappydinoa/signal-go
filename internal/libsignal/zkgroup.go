package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

const (
	// GroupMasterKeyLen is the size of a Groups v2 master key.
	GroupMasterKeyLen = C.SignalGROUP_MASTER_KEY_LEN
	// GroupSecretParamsLen is the serialized GroupSecretParams size.
	GroupSecretParamsLen = C.SignalGROUP_SECRET_PARAMS_LEN
	// GroupPublicParamsLen is the serialized GroupPublicParams size.
	GroupPublicParamsLen = C.SignalGROUP_PUBLIC_PARAMS_LEN
	// GroupIdentifierLen is the 32-byte group identifier used in TypingMessage.groupId.
	GroupIdentifierLen = C.SignalGROUP_IDENTIFIER_LEN
	// UUIDCiphertextLen is the encrypted service id ciphertext size.
	UUIDCiphertextLen = C.SignalUUID_CIPHERTEXT_LEN
	// ZKRandomnessLen is the randomness size for zkgroup deterministic ops.
	ZKRandomnessLen = C.SignalRANDOMNESS_LEN
)

// Production ZK group server public params (Signal production).
// See libsignal rust/net/chat/examples/fetch_profile_key_credential.rs.
var productionServerPublicParamsB64 = "AMhf5ywVwITZMsff/eCyudZx9JDmkkkbV6PInzG4p8x3VqVJSFiMvnvlEKWuRob/1eaIetR31IYeAbm0NdOuHH8Qi+Rexi1wLlpzIo1gstHWBfZzy1+qHRV5A4TqPp15YzBPm0WSggW6PbSn+F4lf57VCnHF7p8SvzAA2ZZJPYJURt8X7bbg+H3i+PEjH9DXItNEqs2sNcug37xZQDLm7X36nOoGPs54XsEGzPdEV+itQNGUFEjY6X9Uv+Acuks7NpyGvCoKxGwgKgE5XyJ+nNKlyHHOLb6N1NuHyBrZrgtY/JYJHRooo5CEqYKBqdFnmbTVGEkCvJKxLnjwKWf+fEPoWeQFj5ObDjcKMZf2Jm2Ae69x+ikU5gBXsRmoF94GXTLfN0/vLt98KDPnxwAQL9j5V1jGOY8jQl6MLxEs56cwXN0dqCnImzVH3TZT1cJ8SW1BRX6qIVxEzjsSGx3yxF3suAilPMqGRp4ffyopjMD1JXiKR2RwLKzizUe5e8XyGOy9fplzhw3jVzTRyUZTRSZKkMLWcQ/gv0E4aONNqs4P+NameAZYOD12qRkxosQQP5uux6B2nRyZ7sAV54DgFyLiRcq1FvwKw2EPQdk4HDoePrO/RNUbyNddnM/mMgj4FW65xCoT1LmjrIjsv/Ggdlx46ueczhMgtBunx1/w8k8V+l8LVZ8gAT6wkU5J+DPQalQguMg12Jzug3q4TbdHiGCmD9EunCwOmsLuLJkz6EcSYXtrlDEnAM+hicw7iergYLLlMXpfTdGxJCWJmP4zqUFeTTmsmhsjGBt7NiEB/9pFFEB3pSbf4iiUukw63Eo8Aqnf4iwob6X1QviCWuc8t0LUlT9vALgh/f2DPVOOmR0RW6bgRvc7DSF20V/omg+YBw=="

// ServerPublicParams is a deserialized zkgroup server public parameter set.
type ServerPublicParams struct {
	raw C.SignalMutPointerServerPublicParams
}

func (p *ServerPublicParams) constPtr() C.SignalConstPointerServerPublicParams {
	return C.SignalConstPointerServerPublicParams{raw: p.raw.raw}
}

func wrapServerPublicParams(raw C.SignalMutPointerServerPublicParams) *ServerPublicParams {
	p := &ServerPublicParams{raw: raw}
	runtime.SetFinalizer(p, func(p *ServerPublicParams) {
		_ = checkError(C.signal_server_public_params_destroy(p.raw))
	})
	return p
}

// ProductionServerPublicParams returns the well-known Signal production
// zkgroup server public params.
func ProductionServerPublicParams() (*ServerPublicParams, error) {
	b, err := base64.StdEncoding.DecodeString(productionServerPublicParamsB64)
	if err != nil {
		return nil, fmt.Errorf("libsignal.ProductionServerPublicParams: decode: %w", err)
	}
	return DeserializeServerPublicParams(b)
}

// DeserializeServerPublicParams parses serialized server public params.
func DeserializeServerPublicParams(b []byte) (*ServerPublicParams, error) {
	if len(b) == 0 {
		return nil, errors.New("libsignal.DeserializeServerPublicParams: empty input")
	}
	var out C.SignalMutPointerServerPublicParams
	if err := checkError(C.signal_server_public_params_deserialize(&out, borrowed(b))); err != nil {
		return nil, err
	}
	keepAlive(b)
	return wrapServerPublicParams(out), nil
}

// GroupSecretParamsFromMasterKey derives group secret params from a 32-byte
// master key.
func GroupSecretParamsFromMasterKey(masterKey []byte) ([GroupSecretParamsLen]byte, error) {
	var out [GroupSecretParamsLen]byte
	if len(masterKey) != GroupMasterKeyLen {
		return out, fmt.Errorf("libsignal.GroupSecretParamsFromMasterKey: length %d, want %d", len(masterKey), GroupMasterKeyLen)
	}
	if err := checkError(C.signal_group_master_key_check_valid_contents(borrowed(masterKey))); err != nil {
		return out, err
	}
	keepAlive(masterKey)
	if err := checkError(C.signal_group_secret_params_derive_from_master_key(
		cGroupSecretParamsOut(&out),
		cGroupMasterKeyIn(masterKey),
	)); err != nil {
		return out, err
	}
	return out, nil
}

// GroupIdentifierFromMasterKey derives the 32-byte group identifier carried
// in TypingMessage.groupId from a Groups v2 master key.
func GroupIdentifierFromMasterKey(masterKey []byte) ([GroupIdentifierLen]byte, error) {
	var out [GroupIdentifierLen]byte
	secretParams, err := GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return out, err
	}
	publicParams, err := GroupSecretParamsPublicParams(secretParams)
	if err != nil {
		return out, err
	}
	if err := checkError(C.signal_group_public_params_get_group_identifier(
		cGroupIdentifierOut(&out),
		cGroupPublicParamsIn(&publicParams),
	)); err != nil {
		return out, err
	}
	return out, nil
}

// GroupSecretParamsPublicParams returns the public params for secret params.
func GroupSecretParamsPublicParams(secretParams [GroupSecretParamsLen]byte) ([GroupPublicParamsLen]byte, error) {
	var out [GroupPublicParamsLen]byte
	if err := checkError(C.signal_group_secret_params_get_public_params(
		cGroupPublicParamsOut(&out),
		cGroupSecretParamsIn(&secretParams),
	)); err != nil {
		return out, err
	}
	return out, nil
}

// GroupSecretParamsDecryptBlob decrypts a padded group attribute blob.
func GroupSecretParamsDecryptBlob(secretParams [GroupSecretParamsLen]byte, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_group_secret_params_decrypt_blob_with_padding(
		&buf,
		cGroupSecretParamsIn(&secretParams),
		borrowed(ciphertext),
	)); err != nil {
		return nil, err
	}
	keepAlive(ciphertext)
	return goBytesFromOwnedBuffer(buf), nil
}

// GroupSecretParamsEncryptBlob encrypts a group attribute blob with random
// padding. Used in tests to build encrypted wire fixtures.
func GroupSecretParamsEncryptBlob(secretParams [GroupSecretParamsLen]byte, plaintext []byte, randomness [ZKRandomnessLen]byte) ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_group_secret_params_encrypt_blob_with_padding_deterministic(
		&buf,
		cGroupSecretParamsIn(&secretParams),
		cRandomnessIn(&randomness),
		borrowed(plaintext),
		0,
	)); err != nil {
		return nil, err
	}
	keepAlive(plaintext)
	return goBytesFromOwnedBuffer(buf), nil
}

// GroupSecretParamsDecryptServiceID decrypts a UUID ciphertext to a service id.
func GroupSecretParamsDecryptServiceID(secretParams [GroupSecretParamsLen]byte, ciphertext []byte) (ServiceIDFixedWidth, error) {
	var out ServiceIDFixedWidth
	if len(ciphertext) != UUIDCiphertextLen {
		return out, fmt.Errorf("libsignal.GroupSecretParamsDecryptServiceID: length %d, want %d", len(ciphertext), UUIDCiphertextLen)
	}
	var cout C.SignalServiceIdFixedWidthBinaryBytes
	if err := checkError(C.signal_group_secret_params_decrypt_service_id(
		&cout,
		cGroupSecretParamsIn(&secretParams),
		cUUIDCiphertextIn(ciphertext),
	)); err != nil {
		return out, err
	}
	return serviceIDFromC(cout), nil
}

// ReceiveAuthCredentialWithPni converts a server auth credential response into
// a stored auth credential for the given ACI/PNI and redemption day.
func (p *ServerPublicParams) ReceiveAuthCredentialWithPni(
	aci, pni ServiceIDFixedWidth,
	redemptionTimeSeconds uint64,
	response []byte,
) ([]byte, error) {
	if len(response) == 0 {
		return nil, errors.New("libsignal.ReceiveAuthCredentialWithPni: empty response")
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_server_public_params_receive_auth_credential_with_pni_as_service_id(
		&buf,
		p.constPtr(),
		cServiceID(aci),
		cServiceID(pni),
		C.uint64_t(redemptionTimeSeconds),
		borrowed(response),
	)); err != nil {
		return nil, err
	}
	keepAlive(response)
	runtime.KeepAlive(p)
	return goBytesFromOwnedBuffer(buf), nil
}

// CreateAuthCredentialPresentation builds a zero-knowledge auth presentation
// for Groups v2 REST calls.
func (p *ServerPublicParams) CreateAuthCredentialPresentation(
	groupSecretParams [GroupSecretParamsLen]byte,
	authCredential []byte,
	randomness [ZKRandomnessLen]byte,
) ([]byte, error) {
	if len(authCredential) == 0 {
		return nil, errors.New("libsignal.CreateAuthCredentialPresentation: empty credential")
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_server_public_params_create_auth_credential_with_pni_presentation_deterministic(
		&buf,
		p.constPtr(),
		cRandomnessIn(&randomness),
		cGroupSecretParamsIn(&groupSecretParams),
		borrowed(authCredential),
	)); err != nil {
		return nil, err
	}
	keepAlive(authCredential)
	runtime.KeepAlive(p)
	return goBytesFromOwnedBuffer(buf), nil
}

// Randomness returns 32 bytes of CSPRNG randomness for zkgroup ops.
func Randomness() ([ZKRandomnessLen]byte, error) {
	var out [ZKRandomnessLen]byte
	if _, err := rand.Read(out[:]); err != nil {
		return out, fmt.Errorf("libsignal.Randomness: %w", err)
	}
	return out, nil
}

// GroupSecretParamsEncryptServiceID encrypts a service id to UUID ciphertext.
func GroupSecretParamsEncryptServiceID(secretParams [GroupSecretParamsLen]byte, serviceID ServiceIDFixedWidth) ([UUIDCiphertextLen]byte, error) {
	var out [UUIDCiphertextLen]byte
	if err := checkError(C.signal_group_secret_params_encrypt_service_id(
		cUUIDCiphertextOut(&out),
		cGroupSecretParamsIn(&secretParams),
		cServiceID(serviceID),
	)); err != nil {
		return out, err
	}
	return out, nil
}

// GroupsV2AuthorizationHeader builds the HTTP Basic authorization header for
// Groups v2 storage requests. Username is hex(group public params); password
// is hex(auth credential presentation).
func GroupsV2AuthorizationHeader(publicParams [GroupPublicParamsLen]byte, presentation []byte) string {
	user := hex.EncodeToString(publicParams[:])
	pass := hex.EncodeToString(presentation)
	raw := user + ":" + pass
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

func cGroupMasterKeyIn(b []byte) *[C.SignalGROUP_MASTER_KEY_LEN]C.uchar {
	return (*[C.SignalGROUP_MASTER_KEY_LEN]C.uchar)(unsafe.Pointer(&b[0]))
}

func cGroupSecretParamsIn(b *[GroupSecretParamsLen]byte) *[C.SignalGROUP_SECRET_PARAMS_LEN]C.uchar {
	return (*[C.SignalGROUP_SECRET_PARAMS_LEN]C.uchar)(unsafe.Pointer(b))
}

func cGroupSecretParamsOut(b *[GroupSecretParamsLen]byte) *[C.SignalGROUP_SECRET_PARAMS_LEN]C.uchar {
	return (*[C.SignalGROUP_SECRET_PARAMS_LEN]C.uchar)(unsafe.Pointer(b))
}

func cGroupPublicParamsOut(b *[GroupPublicParamsLen]byte) *[C.SignalGROUP_PUBLIC_PARAMS_LEN]C.uchar {
	return (*[C.SignalGROUP_PUBLIC_PARAMS_LEN]C.uchar)(unsafe.Pointer(b))
}

func cGroupPublicParamsIn(b *[GroupPublicParamsLen]byte) *[C.SignalGROUP_PUBLIC_PARAMS_LEN]C.uchar {
	return (*[C.SignalGROUP_PUBLIC_PARAMS_LEN]C.uchar)(unsafe.Pointer(b))
}

func cGroupIdentifierOut(b *[GroupIdentifierLen]byte) *[C.SignalGROUP_IDENTIFIER_LEN]C.uchar {
	return (*[C.SignalGROUP_IDENTIFIER_LEN]C.uchar)(unsafe.Pointer(b))
}

func cUUIDCiphertextIn(b []byte) *[C.SignalUUID_CIPHERTEXT_LEN]C.uchar {
	return (*[C.SignalUUID_CIPHERTEXT_LEN]C.uchar)(unsafe.Pointer(&b[0]))
}

func cUUIDCiphertextOut(b *[UUIDCiphertextLen]byte) *[C.SignalUUID_CIPHERTEXT_LEN]C.uchar {
	return (*[C.SignalUUID_CIPHERTEXT_LEN]C.uchar)(unsafe.Pointer(b))
}

func cRandomnessIn(b *[ZKRandomnessLen]byte) *[C.SignalRANDOMNESS_LEN]C.uint8_t {
	return (*[C.SignalRANDOMNESS_LEN]C.uint8_t)(unsafe.Pointer(b))
}
