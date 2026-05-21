package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchPreKeyBundleSendsExpectedRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(FetchPreKeyResponse{
			IdentityKey: base64.StdEncoding.EncodeToString([]byte{0x05}),
			Devices: []BundleDevice{
				{DeviceID: 1, RegistrationID: 4242},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	creds := Credentials{Username: "u", Password: "p"}
	resp, err := c.FetchPreKeyBundle(context.Background(), creds, "aci-uuid", "*")
	if err != nil {
		t.Fatalf("FetchPreKeyBundle: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v2/keys/aci-uuid/*" {
		t.Errorf("method/path: %s %s", gotMethod, gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("auth = %q", gotAuth)
	}
	if len(resp.Devices) != 1 || resp.Devices[0].RegistrationID != 4242 {
		t.Errorf("decoded resp: %+v", resp)
	}
}

func TestFetchPreKeyBundleValidatesInputs(t *testing.T) {
	c := New("https://example.com", "test")
	if _, err := c.FetchPreKeyBundle(context.Background(), Credentials{}, "aci", "1"); err == nil {
		t.Error("expected error empty creds")
	}
	if _, err := c.FetchPreKeyBundle(context.Background(), Credentials{Username: "u", Password: "p"}, "", "1"); err == nil {
		t.Error("expected error empty serviceID")
	}
}

func TestSendMessageSendsExpectedRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotBody SendMessageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_ = json.NewEncoder(w).Encode(SendMessageResponse{NeedsSync: true})
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	creds := Credentials{Username: "aci-uuid.1", Password: "pw"}
	req := SendMessageRequest{
		Timestamp: 1700000000000,
		Urgent:    true,
		Messages: []MessageEnvelope{
			{Type: CiphertextTypePreKey, DestinationDeviceID: 1, DestinationRegistrationID: 4242, Content: "AA=="},
		},
	}
	resp, err := c.SendMessage(context.Background(), creds, "bob-aci", req)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !resp.NeedsSync {
		t.Errorf("NeedsSync not propagated")
	}
	if gotMethod != http.MethodPut || gotPath != "/v1/messages/bob-aci" {
		t.Errorf("method/path: %s %s", gotMethod, gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotBody.Timestamp != 1700000000000 || gotBody.Messages[0].Type != CiphertextTypePreKey {
		t.Errorf("server saw: %+v", gotBody)
	}
}

func TestSendMessageMapsMismatchedDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"missingDevices": []uint32{2, 3},
			"extraDevices":   []uint32{},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	_, err := c.SendMessage(context.Background(), Credentials{Username: "u", Password: "p"}, "bob",
		SendMessageRequest{Timestamp: 1, Messages: []MessageEnvelope{{Type: 1, DestinationDeviceID: 1, Content: "AA=="}}})
	var mde *MismatchedDevicesError
	if !errors.As(err, &mde) {
		t.Fatalf("err = %v (%T), want *MismatchedDevicesError", err, err)
	}
	if len(mde.MissingDevices) != 2 || mde.MissingDevices[0] != 2 {
		t.Errorf("MissingDevices = %v", mde.MissingDevices)
	}
}

func TestSendMessageMapsStaleDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"staleDevices": []uint32{2},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	_, err := c.SendMessage(context.Background(), Credentials{Username: "u", Password: "p"}, "bob",
		SendMessageRequest{Timestamp: 1, Messages: []MessageEnvelope{{Type: 1, DestinationDeviceID: 1, Content: "AA=="}}})
	var sde *StaleDevicesError
	if !errors.As(err, &sde) {
		t.Fatalf("err = %v (%T), want *StaleDevicesError", err, err)
	}
	if len(sde.StaleDevices) != 1 || sde.StaleDevices[0] != 2 {
		t.Errorf("StaleDevices = %v", sde.StaleDevices)
	}
}

func TestSendMessagePassesThroughOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(srv.URL, "test")
	_, err := c.SendMessage(context.Background(), Credentials{Username: "u", Password: "p"}, "bob",
		SendMessageRequest{Timestamp: 1, Messages: []MessageEnvelope{{Type: 1, DestinationDeviceID: 1, Content: "AA=="}}})
	var werr *Error
	if !errors.As(err, &werr) || werr.StatusCode != 400 {
		t.Errorf("err = %v, want *web.Error with status 400", err)
	}
}

func TestSendMessageValidatesInputs(t *testing.T) {
	c := New("https://example.com", "test")
	cases := []struct {
		name string
		fn   func() error
	}{
		{"no creds", func() error {
			_, e := c.SendMessage(context.Background(), Credentials{}, "bob",
				SendMessageRequest{Timestamp: 1, Messages: []MessageEnvelope{{Type: 1}}})
			return e
		}},
		{"no recipient", func() error {
			_, e := c.SendMessage(context.Background(), Credentials{Username: "u", Password: "p"}, "",
				SendMessageRequest{Timestamp: 1, Messages: []MessageEnvelope{{Type: 1}}})
			return e
		}},
		{"no messages", func() error {
			_, e := c.SendMessage(context.Background(), Credentials{Username: "u", Password: "p"}, "bob",
				SendMessageRequest{Timestamp: 1})
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestDecodeBase64HandlesBothEncodings(t *testing.T) {
	std := base64.StdEncoding.EncodeToString([]byte{0xfe, 0xff})
	url := base64.URLEncoding.EncodeToString([]byte{0xfe, 0xff})
	for _, in := range []string{std, url} {
		got, err := DecodeBase64(in)
		if err != nil {
			t.Errorf("DecodeBase64(%q): %v", in, err)
			continue
		}
		if len(got) != 2 || got[0] != 0xfe {
			t.Errorf("DecodeBase64(%q) = %x", in, got)
		}
	}
	if _, err := DecodeBase64("!!!not-base64!!!"); err == nil {
		t.Error("expected error on garbage")
	}
}
