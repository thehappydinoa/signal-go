package signal

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/profile"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
)

func TestFetchProfileDecryptsName(t *testing.T) {
	profileKey := bytes.Repeat([]byte{0x11}, 32)
	aci := "9d0652a3-dcc3-4d11-975f-74d61598733f"
	version, err := libsignal.ProfileKeyVersion(profileKey, aci)
	if err != nil {
		t.Fatalf("ProfileKeyVersion: %v", err)
	}
	uak, err := libsignal.DeriveAccessKey(profileKey)
	if err != nil {
		t.Fatalf("DeriveAccessKey: %v", err)
	}

	nonce := bytes.Repeat([]byte{0x01}, 12)
	nameCT, err := profile.EncryptStringForTest(profileKey, "Bot\x00User", nonce)
	if err != nil {
		t.Fatalf("encrypt name: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/v1/profile/" + aci + "/" + version
		if r.URL.Path != want {
			t.Errorf("path = %s, want %s", r.URL.Path, want)
		}
		if r.Header.Get("Unidentified-Access-Key") != base64.StdEncoding.EncodeToString(uak[:]) {
			http.Error(w, "bad uak", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"` + base64.StdEncoding.EncodeToString(nameCT) + `"}`))
	}))
	defer srv.Close()

	c := newSenderClient(t, srv.URL)
	got, err := c.FetchProfile(context.Background(), aci, profileKey)
	if err != nil {
		t.Fatalf("FetchProfile: %v", err)
	}
	if got.GivenName != "Bot" || got.FamilyName != "User" {
		t.Errorf("name = %q / %q", got.GivenName, got.FamilyName)
	}
	if got.DisplayName() != "Bot User" {
		t.Errorf("DisplayName = %q", got.DisplayName())
	}

	c.mu.Lock()
	cached := append([]byte(nil), c.knownUAKs[aci]...)
	c.mu.Unlock()
	if !bytes.Equal(cached, uak[:]) {
		t.Errorf("cached UAK = %x, want %x", cached, uak[:])
	}
}

func TestSetRecipientProfileKeyDerivesUAK(t *testing.T) {
	c := newSenderClient(t, "")
	key := bytes.Repeat([]byte{0x22}, 32)
	aci := testRecipientACI
	c.SetRecipientProfileKey(aci, key)
	want, err := libsignal.DeriveAccessKey(key)
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	got := append([]byte(nil), c.knownUAKs[aci]...)
	c.mu.Unlock()
	if !bytes.Equal(got, want[:]) {
		t.Errorf("UAK = %x, want %x", got, want[:])
	}
}

func TestInboundProfileKeyPopulatesUAK(t *testing.T) {
	c := newSenderClient(t, "")
	key := bytes.Repeat([]byte{0x33}, 32)
	ts := uint64(time.Now().UnixMilli())
	body := "hi"
	dm := &sspb.DataMessage{
		Body:       &body,
		Timestamp:  &ts,
		ProfileKey: key,
	}
	c.handleDataMessage(testRecipientACI, 1, time.Now(), time.Now(), dm)
	want, _ := libsignal.DeriveAccessKey(key)
	c.mu.Lock()
	got := append([]byte(nil), c.knownUAKs[testRecipientACI]...)
	c.mu.Unlock()
	if !bytes.Equal(got, want[:]) {
		t.Errorf("UAK = %x, want %x", got, want[:])
	}
}
