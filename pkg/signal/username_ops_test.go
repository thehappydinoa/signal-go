package signal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/web"
	"github.com/thehappydinoa/signal-go/pkg/signal/username"
)

func newUsernameClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{
		acct: testAccount(),
		webc: web.New(srv.URL, "test"),
	}
}

func TestPublishedUsernameLinkShareURL(t *testing.T) {
	entropy := bytes.Repeat([]byte{0xAB}, 32)
	p := PublishedUsernameLink{Handle: "handle-uuid", Entropy: entropy}
	got := p.ShareURL()
	if !strings.HasPrefix(got, "https://signal.me/#eu/handle-uuid/") {
		t.Fatalf("ShareURL prefix wrong: %q", got)
	}
	// The entropy encoding must be unpadded url-safe base64.
	wantEnc := base64.RawURLEncoding.EncodeToString(entropy)
	if !strings.HasSuffix(got, wantEnc) {
		t.Fatalf("ShareURL entropy suffix wrong: %q", got)
	}

	// Empty entropy or handle → empty URL (defensive).
	if (PublishedUsernameLink{Handle: "h"}).ShareURL() != "" {
		t.Fatal("expected empty URL for empty entropy")
	}
	if (PublishedUsernameLink{Entropy: entropy}).ShareURL() != "" {
		t.Fatal("expected empty URL for empty handle")
	}
}

func TestReserveUsername_PicksMatchingCandidate(t *testing.T) {
	// Build two real candidates so their hashes are valid.
	cs, err := username.Candidates("alice", 0, 0)
	if err != nil || len(cs) < 2 {
		t.Fatalf("Candidates: %v (n=%d)", err, len(cs))
	}
	// Server will "pick" the second candidate's hash.
	pickedHash, err := cs[1].Hash()
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	cli := newUsernameClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/username_hash/reserve" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"usernameHash": base64.RawURLEncoding.EncodeToString(pickedHash[:]),
		})
	})
	got, err := cli.ReserveUsername(context.Background(), cs[:2])
	if err != nil {
		t.Fatalf("ReserveUsername: %v", err)
	}
	if got != cs[1] {
		t.Fatalf("ReserveUsername picked %q, want %q", got, cs[1])
	}
}

func TestReserveUsername_UnknownHashIsError(t *testing.T) {
	cs, _ := username.Candidates("alice", 0, 0)
	cli := newUsernameClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Reply with a random hash that didn't come from the input list.
		bogus := bytes.Repeat([]byte{0xFF}, libsignal.UsernameHashLen)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"usernameHash": base64.RawURLEncoding.EncodeToString(bogus),
		})
	})
	if _, err := cli.ReserveUsername(context.Background(), cs[:1]); err == nil {
		t.Fatal("expected rejection of out-of-list hash")
	}
}

func TestReserveUsername_EmptyRejected(t *testing.T) {
	cli := &Client{acct: testAccount(), webc: web.New("http://x", "t")}
	if _, err := cli.ReserveUsername(context.Background(), nil); err == nil {
		t.Fatal("expected error on empty candidate list")
	}
}

func TestConfirmUsername_ReturnsPublishedLink(t *testing.T) {
	cs, _ := username.Candidates("bob", 0, 0)
	reserved := cs[0]
	hash, _ := reserved.Hash()

	cli := newUsernameClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/username_hash/confirm" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"usernameHash":       base64.RawURLEncoding.EncodeToString(hash[:]),
			"usernameLinkHandle": "the-handle-uuid",
		})
	})
	got, err := cli.ConfirmUsername(context.Background(), reserved)
	if err != nil {
		t.Fatalf("ConfirmUsername: %v", err)
	}
	if got.Handle != "the-handle-uuid" {
		t.Fatalf("Handle = %q", got.Handle)
	}
	if len(got.Entropy) != 32 {
		t.Fatalf("Entropy length = %d, want 32", len(got.Entropy))
	}
	if !strings.Contains(got.ShareURL(), "the-handle-uuid") {
		t.Fatalf("ShareURL missing handle: %q", got.ShareURL())
	}
}

func TestDeleteUsername(t *testing.T) {
	cli := newUsernameClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/accounts/username_hash" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := cli.DeleteUsername(context.Background()); err != nil {
		t.Fatalf("DeleteUsername: %v", err)
	}
}

func TestRotateUsernameLink(t *testing.T) {
	cli := newUsernameClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/username_link" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"usernameLinkHandle": "rotated-handle",
		})
	})
	got, err := cli.RotateUsernameLink(context.Background(), "carol.98765", true)
	if err != nil {
		t.Fatalf("RotateUsernameLink: %v", err)
	}
	if got.Handle != "rotated-handle" {
		t.Fatalf("handle = %q", got.Handle)
	}
}

func TestDeleteUsernameLink(t *testing.T) {
	cli := newUsernameClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/accounts/username_link" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := cli.DeleteUsernameLink(context.Background()); err != nil {
		t.Fatalf("DeleteUsernameLink: %v", err)
	}
}

func TestLookupUsernameHash(t *testing.T) {
	var hash [HashLen]byte
	for i := range hash {
		hash[i] = byte(i)
	}
	cli := newUsernameClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("lookup must be unauthenticated")
		}
		expected := base64.RawURLEncoding.EncodeToString(hash[:])
		if r.URL.Path != "/v1/accounts/username_hash/"+expected {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"aci-of-owner"}`))
	})
	got, err := cli.LookupUsernameHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("LookupUsernameHash: %v", err)
	}
	if got != "aci-of-owner" {
		t.Fatalf("ACI = %q", got)
	}
}

func TestLookupUsername_FetchDecryptRoundTrip(t *testing.T) {
	// Build a real username link locally, then have the server return
	// the ciphertext under a given handle. LookupUsername decrypts.
	const plaintext = "dan.55555"
	const handle = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	link, err := username.NewLink(plaintext)
	if err != nil {
		t.Fatalf("NewLink: %v", err)
	}
	cli := newUsernameClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("lookup must be unauthenticated")
		}
		if r.URL.Path != "/v1/accounts/username_link/"+handle {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"usernameLinkEncryptedValue": base64.RawURLEncoding.EncodeToString(link.Ciphertext),
		})
	})
	got, err := cli.LookupUsername(context.Background(), handle, link.Entropy)
	if err != nil {
		t.Fatalf("LookupUsername: %v", err)
	}
	if string(got) != plaintext {
		t.Fatalf("decrypt mismatch: got %q, want %q", got, plaintext)
	}
}

func TestLookupUsername_ValidatesInputs(t *testing.T) {
	cli := &Client{acct: testAccount(), webc: web.New("http://x", "t")}
	if _, err := cli.LookupUsername(context.Background(), "", make([]byte, 32)); err == nil {
		t.Fatal("expected empty handle error")
	}
	if _, err := cli.LookupUsername(context.Background(), "../../attack", make([]byte, 32)); err == nil {
		t.Fatal("expected path-injection handle to be rejected")
	}
	if _, err := cli.LookupUsername(context.Background(), "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", []byte{1, 2, 3}); err == nil {
		t.Fatal("expected bad entropy length error")
	}
}
