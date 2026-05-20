package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLinkDeviceSendsExpectedRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotBody LinkDeviceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		resp := LinkDeviceResponse{UUID: "aci-uuid", DeviceID: 7, PNI: "pni-uuid"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	req := LinkDeviceRequest{
		VerificationCode: "code-xyz",
		AccountAttributes: AccountAttributes{
			FetchesMessages:       true,
			RegistrationID:        1234,
			PNIRegistrationID:     5678,
			Capabilities:          DefaultCapabilities(),
			UnidentifiedAccessKey: base64.StdEncoding.EncodeToString(make([]byte, 16)),
		},
		ACISignedPreKey:       SignedECPreKey{KeyID: 1, PublicKey: "AA", Signature: "BB"},
		PNISignedPreKey:       SignedECPreKey{KeyID: 2, PublicKey: "CC", Signature: "DD"},
		ACIPqLastResortPreKey: SignedKEMPreKey{KeyID: 3, PublicKey: "EE", Signature: "FF"},
		PNIPqLastResortPreKey: SignedKEMPreKey{KeyID: 4, PublicKey: "GG", Signature: "HH"},
	}
	resp, err := c.LinkDevice(context.Background(), "code-xyz", "passw0rd", req)
	if err != nil {
		t.Fatalf("LinkDevice: %v", err)
	}
	if resp.DeviceID != 7 || resp.UUID != "aci-uuid" {
		t.Errorf("resp = %+v", resp)
	}
	if gotMethod != http.MethodPut || gotPath != "/v1/devices/link" {
		t.Errorf("method/path: %s %s", gotMethod, gotPath)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("code-xyz:passw0rd"))
	if gotAuth != wantAuth {
		t.Errorf("auth = %q, want %q", gotAuth, wantAuth)
	}
	if gotBody.VerificationCode != "code-xyz" || gotBody.AccountAttributes.RegistrationID != 1234 {
		t.Errorf("server saw %+v", gotBody)
	}
	if gotBody.ACISignedPreKey.KeyID != 1 || gotBody.PNIPqLastResortPreKey.KeyID != 4 {
		t.Errorf("server saw prekeys %+v", gotBody)
	}
}

func TestLinkDeviceSurfacesServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad code", http.StatusForbidden)
	}))
	defer srv.Close()
	c := New(srv.URL, "test")
	_, err := c.LinkDevice(context.Background(), "code", "pwd", LinkDeviceRequest{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("err = %v", err)
	}
}

func TestLinkDeviceValidatesInputs(t *testing.T) {
	c := New("https://example.com", "test")
	if _, err := c.LinkDevice(context.Background(), "", "pw", LinkDeviceRequest{}); err == nil {
		t.Error("expected error empty code")
	}
	if _, err := c.LinkDevice(context.Background(), "code", "", LinkDeviceRequest{}); err == nil {
		t.Error("expected error empty password")
	}
}
