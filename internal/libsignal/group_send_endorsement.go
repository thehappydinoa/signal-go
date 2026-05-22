package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// GroupSendEndorsementsResponseExpiration returns the expiration timestamp
// (seconds since epoch) embedded in a server GSE response.
func GroupSendEndorsementsResponseExpiration(response []byte) (uint64, error) {
	if len(response) == 0 {
		return 0, errors.New("libsignal.GroupSendEndorsementsResponseExpiration: empty response")
	}
	var exp C.uint64_t
	if err := checkError(C.signal_group_send_endorsements_response_get_expiration(
		&exp,
		borrowed(response),
	)); err != nil {
		return 0, err
	}
	keepAlive(response)
	return uint64(exp), nil
}

// ReceiveGroupSendEndorsementsWithServiceIDs validates a server GSE response
// and returns one endorsement blob per member in memberACIs order.
func ReceiveGroupSendEndorsementsWithServiceIDs(
	response []byte,
	memberACIs []ServiceIDFixedWidth,
	localUser ServiceIDFixedWidth,
	secretParams [GroupSecretParamsLen]byte,
	serverParams *ServerPublicParams,
	when time.Time,
) ([][]byte, error) {
	if len(response) == 0 {
		return nil, errors.New("libsignal.ReceiveGroupSendEndorsementsWithServiceIDs: empty response")
	}
	if len(memberACIs) == 0 {
		return nil, errors.New("libsignal.ReceiveGroupSendEndorsementsWithServiceIDs: no members")
	}
	if serverParams == nil {
		return nil, errors.New("libsignal.ReceiveGroupSendEndorsementsWithServiceIDs: nil server params")
	}

	membersWire := concatServiceIDs(memberACIs)
	var out C.SignalBytestringArray
	err := checkError(C.signal_group_send_endorsements_response_receive_and_combine_with_service_ids(
		&out,
		borrowed(response),
		borrowed(membersWire),
		cServiceID(localUser),
		C.uint64_t(when.Unix()),
		cGroupSecretParamsIn(&secretParams),
		serverParams.constPtr(),
	))
	keepAlive(response)
	keepAlive(membersWire)
	runtime.KeepAlive(serverParams)
	if err != nil {
		return nil, err
	}
	return goBytestringArrayFromC(out), nil
}

// CombineGroupSendEndorsements merges individual member endorsements into one
// blob suitable for [GroupSendEndorsementToFullToken].
func CombineGroupSendEndorsements(endorsements ...[]byte) ([]byte, error) {
	if len(endorsements) == 0 {
		return nil, errors.New("libsignal.CombineGroupSendEndorsements: empty input")
	}
	ptrs := make([]C.SignalBorrowedBuffer, len(endorsements))
	for i, e := range endorsements {
		if len(e) == 0 {
			return nil, fmt.Errorf("libsignal.CombineGroupSendEndorsements: empty endorsement at index %d", i)
		}
		ptrs[i] = borrowed(e)
	}
	slice := C.SignalBorrowedSliceOfBuffers{
		base:   (*C.SignalBorrowedBuffer)(unsafe.Pointer(&ptrs[0])),
		length: C.size_t(len(ptrs)),
	}
	var buf C.SignalOwnedBuffer
	err := checkError(C.signal_group_send_endorsement_combine(&buf, slice))
	for _, e := range endorsements {
		keepAlive(e)
	}
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// GroupSendEndorsementToFullToken converts a combined endorsement to the
// wire token placed in the Group-Send-Token HTTP header.
func GroupSendEndorsementToFullToken(
	combined []byte,
	secretParams [GroupSecretParamsLen]byte,
	expiration time.Time,
) ([]byte, error) {
	if len(combined) == 0 {
		return nil, errors.New("libsignal.GroupSendEndorsementToFullToken: empty endorsement")
	}
	var tokenBuf C.SignalOwnedBuffer
	if err := checkError(C.signal_group_send_endorsement_to_token(
		&tokenBuf,
		borrowed(combined),
		cGroupSecretParamsIn(&secretParams),
	)); err != nil {
		return nil, err
	}
	keepAlive(combined)
	token := goBytesFromOwnedBuffer(tokenBuf)

	var fullBuf C.SignalOwnedBuffer
	if err := checkError(C.signal_group_send_token_to_full_token(
		&fullBuf,
		borrowed(token),
		C.uint64_t(expiration.Unix()),
	)); err != nil {
		return nil, err
	}
	keepAlive(token)
	return goBytesFromOwnedBuffer(fullBuf), nil
}

func concatServiceIDs(ids []ServiceIDFixedWidth) []byte {
	out := make([]byte, 0, len(ids)*ServiceIDFixedWidthLen)
	for _, id := range ids {
		out = append(out, id[:]...)
	}
	return out
}
