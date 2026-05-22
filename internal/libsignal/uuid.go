package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// ParseUUIDString parses a canonical 8-4-4-4-12 UUID string into 16 bytes.
func ParseUUIDString(s string) ([16]byte, error) {
	var out [16]byte
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return out, fmt.Errorf("libsignal.ParseUUIDString: invalid uuid %q", s)
	}
	hexStr := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return out, fmt.Errorf("libsignal.ParseUUIDString: %w", err)
	}
	if len(b) != 16 {
		return out, errors.New("libsignal.ParseUUIDString: decoded length != 16")
	}
	copy(out[:], b)
	return out, nil
}

// NewRandomUUID returns a random version-4 UUID in canonical string form.
func NewRandomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("libsignal.NewRandomUUID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b), nil
}

func uuidToC(id [16]byte) C.SignalUuid {
	var u C.SignalUuid
	for i := 0; i < 16; i++ {
		u.bytes[i] = C.uint8_t(id[i])
	}
	return u
}
