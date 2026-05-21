package libsignal

import "testing"

func TestAddressRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		svc  string
		dev  uint32
	}{
		{"primary aci", "11111111-2222-3333-4444-555555555555", 1},
		{"linked pni", "PNI:aaaa-bbbb-cccc-dddd-eeee", 7},
		{"plain phone fallback", "+15551234567", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := NewAddress(tc.svc, tc.dev)
			if err != nil {
				t.Fatalf("NewAddress: %v", err)
			}
			gotSvc, err := a.ServiceID()
			if err != nil {
				t.Fatalf("ServiceID: %v", err)
			}
			if gotSvc != tc.svc {
				t.Errorf("ServiceID = %q, want %q", gotSvc, tc.svc)
			}
			gotDev, err := a.DeviceID()
			if err != nil {
				t.Fatalf("DeviceID: %v", err)
			}
			if gotDev != tc.dev {
				t.Errorf("DeviceID = %d, want %d", gotDev, tc.dev)
			}
		})
	}
}

func TestNewAddressRejectsEmpty(t *testing.T) {
	if _, err := NewAddress("", 1); err == nil {
		t.Error("expected error on empty service id")
	}
}
