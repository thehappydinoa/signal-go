package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
)

// IncrementalMac computes chunked HMAC-SHA256 digests over a byte stream.
type IncrementalMac struct {
	raw C.SignalMutPointerIncrementalMac
}

// ValidatingMac verifies chunked HMAC-SHA256 digests over a byte stream.
type ValidatingMac struct {
	raw C.SignalMutPointerValidatingMac
}

// CalculateIncrementalMacChunkSize returns the recommended chunk size for
// incremental MAC over data of the given encrypted attachment size.
func CalculateIncrementalMacChunkSize(dataSize uint32) (uint32, error) {
	var out C.uint32_t
	if err := checkError(C.signal_incremental_mac_calculate_chunk_size(&out, C.uint32_t(dataSize))); err != nil {
		return 0, fmt.Errorf("libsignal.CalculateIncrementalMacChunkSize: %w", err)
	}
	return uint32(out), nil
}

// NewIncrementalMac initializes incremental MAC with a 32-byte HMAC key and
// positive chunk size.
func NewIncrementalMac(key []byte, chunkSize uint32) (*IncrementalMac, error) {
	if len(key) != 32 {
		return nil, errors.New("libsignal.NewIncrementalMac: key must be 32 bytes")
	}
	if chunkSize == 0 {
		return nil, errors.New("libsignal.NewIncrementalMac: chunk size must be positive")
	}
	var out C.SignalMutPointerIncrementalMac
	if err := checkError(C.signal_incremental_mac_initialize(&out, borrowed(key), C.uint32_t(chunkSize))); err != nil {
		return nil, err
	}
	keepAlive(key)
	m := &IncrementalMac{raw: out}
	runtime.SetFinalizer(m, (*IncrementalMac).destroy)
	return m, nil
}

// Update feeds bytes into the incremental MAC. Non-empty return values are
// completed chunk digests emitted when a chunk boundary is crossed.
func (m *IncrementalMac) Update(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_incremental_mac_update(&buf, m.raw, borrowed(data), 0, C.uint32_t(len(data)))); err != nil {
		return nil, err
	}
	keepAlive(data)
	out := goBytesFromOwnedBuffer(buf)
	if len(out) == 0 {
		return nil, nil
	}
	return [][]byte{out}, nil
}

// Finalize returns the trailing chunk digest and destroys the handle.
func (m *IncrementalMac) Finalize() ([]byte, error) {
	runtime.SetFinalizer(m, nil)
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_incremental_mac_finalize(&buf, m.raw)); err != nil {
		return nil, err
	}
	m.raw.raw = nil
	return goBytesFromOwnedBuffer(buf), nil
}

func (m *IncrementalMac) destroy() {
	if m.raw.raw != nil {
		C.signal_incremental_mac_destroy(m.raw)
		m.raw.raw = nil
	}
}

// NewValidatingMac initializes incremental MAC verification. digests is the
// concatenation of expected chunk MACs (same order as produced by
// [IncrementalMac]).
func NewValidatingMac(key []byte, chunkSize uint32, digests []byte) (*ValidatingMac, error) {
	if len(key) != 32 {
		return nil, errors.New("libsignal.NewValidatingMac: key must be 32 bytes")
	}
	if chunkSize == 0 {
		return nil, errors.New("libsignal.NewValidatingMac: chunk size must be positive")
	}
	if len(digests) == 0 {
		return nil, errors.New("libsignal.NewValidatingMac: digests required")
	}
	var out C.SignalMutPointerValidatingMac
	if err := checkError(C.signal_validating_mac_initialize(&out, borrowed(key), C.uint32_t(chunkSize), borrowed(digests))); err != nil {
		return nil, err
	}
	keepAlive(key)
	keepAlive(digests)
	v := &ValidatingMac{raw: out}
	runtime.SetFinalizer(v, (*ValidatingMac).destroy)
	return v, nil
}

// Update verifies bytes and returns the number of fully validated bytes.
func (v *ValidatingMac) Update(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	var validated C.int32_t
	if err := checkError(C.signal_validating_mac_update(&validated, v.raw, borrowed(data), 0, C.uint32_t(len(data)))); err != nil {
		return 0, err
	}
	keepAlive(data)
	if validated < 0 {
		return 0, errors.New("libsignal.ValidatingMac.Update: MAC verification failed")
	}
	return int(validated), nil
}

// Finalize completes verification and returns the number of bytes validated
// in the final partial chunk.
func (v *ValidatingMac) Finalize() (int, error) {
	runtime.SetFinalizer(v, nil)
	var validated C.int32_t
	if err := checkError(C.signal_validating_mac_finalize(&validated, v.raw)); err != nil {
		return 0, err
	}
	v.raw.raw = nil
	if validated < 0 {
		return 0, errors.New("libsignal.ValidatingMac.Finalize: MAC verification failed")
	}
	return int(validated), nil
}

func (v *ValidatingMac) destroy() {
	if v.raw.raw != nil {
		C.signal_validating_mac_destroy(v.raw)
		v.raw.raw = nil
	}
}

// DigestIncrementalMac computes the full incremental MAC digest chain for
// data using the given 32-byte MAC key and chunk size.
func DigestIncrementalMac(key []byte, chunkSize uint32, data []byte) ([]byte, error) {
	m, err := NewIncrementalMac(key, chunkSize)
	if err != nil {
		return nil, err
	}
	var parts [][]byte
	for len(data) > 0 {
		n := int(chunkSize)
		if n > len(data) {
			n = len(data)
		}
		chunk := data[:n]
		data = data[n:]
		emitted, err := m.Update(chunk)
		if err != nil {
			return nil, err
		}
		parts = append(parts, emitted...)
	}
	final, err := m.Finalize()
	if err != nil {
		return nil, err
	}
	parts = append(parts, final)
	out := make([]byte, 0, len(parts)*32)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out, nil
}
