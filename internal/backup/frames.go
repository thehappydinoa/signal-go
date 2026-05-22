package backup

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ReadVarintFrame reads one length-delimited protobuf blob from r.
func ReadVarintFrame(r io.ByteReader) ([]byte, error) {
	length, err := readVarint(r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	if length == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, length)
	for i := range buf {
		b, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("backup.ReadVarintFrame: %w", err)
		}
		buf[i] = b
	}
	return buf, nil
}

func readVarint(r io.ByteReader) (int, error) {
	val, _, err := readVarintCounted(r)
	return val, err
}

func readVarintCounted(r io.ByteReader) (int, int, error) {
	var (
		result uint64
		shift  uint
		count  int
	)
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, count, err
		}
		count++
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			if result > uint64(^uint(0)>>1) {
				return 0, count, errors.New("backup: varint overflow")
			}
			return int(result), count, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, count, errors.New("backup: varint too long")
		}
	}
}

// VarintEncodeLength returns the protobuf varint encoding of length.
func VarintEncodeLength(length int) []byte {
	if length < 0 {
		panic("backup.VarintEncodeLength: negative length")
	}
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], uint64(length))
	return append([]byte(nil), buf[:n]...)
}
