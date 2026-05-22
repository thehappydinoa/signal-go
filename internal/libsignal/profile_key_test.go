package libsignal

import (
	"bytes"
	"testing"
)

func TestDeriveAccessKeyKAT(t *testing.T) {
	// Vectors from libsignal rust/zkgroup/src/api/profiles/profile_key.rs
	tests := []struct {
		profileKey []byte
		wantUAK    []byte
	}{
		{
			profileKey: []byte{
				0xb9, 0x50, 0x42, 0xa2, 0xc2, 0xd9, 0xe5, 0xb3, 0xbb, 0x09, 0x30, 0x0e, 0xe4,
				0x08, 0xa1, 0x72, 0xfa, 0xcd, 0x96, 0xe9, 0x1b, 0x50, 0x4e, 0x04, 0x3a, 0x5a,
				0x02, 0x3d, 0xc4, 0xcf, 0xf3, 0x59,
			},
			wantUAK: []byte{
				0x24, 0xfb, 0x96, 0xd4, 0xa5, 0xe3, 0x33, 0xe9, 0xd4, 0x45, 0x12, 0x05, 0xb9,
				0xe2, 0xfa, 0xed,
			},
		},
		{
			profileKey: []byte{
				0x26, 0x19, 0x7b, 0x17, 0xe5, 0xa2, 0xc3, 0x6d, 0x8c, 0x95, 0x18, 0xc3, 0x53,
				0x58, 0xf1, 0x23, 0xc4, 0x76, 0x00, 0x0d, 0xb6, 0xda, 0x75, 0x65, 0xc0, 0xd4,
				0x1f, 0x66, 0x74, 0x46, 0x2c, 0x4d,
			},
			wantUAK: []byte{
				0xe8, 0x95, 0xc3, 0x0c, 0xf7, 0x80, 0x75, 0x7d, 0x22, 0xf7, 0xa1, 0x79, 0x70,
				0x8b, 0x14, 0xa1,
			},
		},
	}
	for _, tc := range tests {
		got, err := DeriveAccessKey(tc.profileKey)
		if err != nil {
			t.Fatalf("DeriveAccessKey: %v", err)
		}
		if !bytes.Equal(got[:], tc.wantUAK) {
			t.Errorf("UAK = %x, want %x", got[:], tc.wantUAK)
		}
	}
}

func TestProfileKeyVersionDeterministic(t *testing.T) {
	key := bytes.Repeat([]byte{0xab}, ProfileKeyLen)
	aci := "9d0652a3-dcc3-4d11-975f-74d61598733f"
	v1, err := ProfileKeyVersion(key, aci)
	if err != nil {
		t.Fatalf("ProfileKeyVersion: %v", err)
	}
	if len(v1) != ProfileKeyVersionEncodedLen {
		t.Fatalf("version len = %d, want %d", len(v1), ProfileKeyVersionEncodedLen)
	}
	v2, err := ProfileKeyVersion(key, aci)
	if err != nil {
		t.Fatalf("ProfileKeyVersion: %v", err)
	}
	if v1 != v2 {
		t.Errorf("version not stable: %q vs %q", v1, v2)
	}
}

func TestParseServiceIDStringACI(t *testing.T) {
	aci := "9d0652a3-dcc3-4d11-975f-74d61598733f"
	_, err := ParseServiceIDString(aci)
	if err != nil {
		t.Fatalf("ParseServiceIDString: %v", err)
	}
}
