package attachment

import "math"

// LogPadSize returns the zero-padded plaintext target size used by Signal
// Desktop's attachment v2 format.
func LogPadSize(size int) int {
	if size <= 0 {
		return 541
	}
	return int(math.Max(541, math.Floor(math.Pow(1.05, math.Ceil(math.Log(float64(size))/math.Log(1.05))))))
}

func logPad(plaintext []byte) []byte {
	target := LogPadSize(len(plaintext))
	if target <= len(plaintext) {
		return append([]byte(nil), plaintext...)
	}
	out := make([]byte, target)
	copy(out, plaintext)
	return out
}

func trimLogPad(padded []byte, plaintextLen int) []byte {
	if plaintextLen <= 0 || plaintextLen > len(padded) {
		return padded
	}
	return padded[:plaintextLen]
}
