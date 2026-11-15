package web

import (
	"bytes"
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

func newUsernameServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL, "test")
}

func authCreds() Credentials { return Credentials{Username: "aci.1", Password: "pw"} }

func TestReserveUsernameHash_Happy(t *testing.T) {
	want := bytes.Repeat([]byte{0xAB}, 32)
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/accounts/username_hash/reserve" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing auth header")
		}
		var body reserveUsernameRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.UsernameHashes) != 2 {
			t.Fatalf("hashes = %d, want 2", len(body.UsernameHashes))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reserveUsernameResponseBody{
			UsernameHash: base64.RawURLEncoding.EncodeToString(want),
		})
	})
	got, err := c.ReserveUsernameHash(context.Background(), authCreds(), [][]byte{
		bytes.Repeat([]byte{1}, 32),
		bytes.Repeat([]byte{2}, 32),
	})
	if err != nil {
		t.Fatalf("ReserveUsernameHash: %v", err)
	}
	if !bytes.Equal(got.UsernameHash, want) {
		t.Fatalf("hash mismatch")
	}
}

func TestReserveUsernameHash_ValidatesInput(t *testing.T) {
	c := New("http://unused", "test")
	cases := []struct {
		name   string
		creds  Credentials
		hashes [][]byte
	}{
		{"no creds", Credentials{}, [][]byte{bytes.Repeat([]byte{1}, 32)}},
		{"zero hashes", authCreds(), nil},
		{"wrong hash length", authCreds(), [][]byte{{1, 2, 3}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.ReserveUsernameHash(context.Background(), tc.creds, tc.hashes); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestReserveUsernameHash_409(t *testing.T) {
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	_, err := c.ReserveUsernameHash(context.Background(), authCreds(), [][]byte{bytes.Repeat([]byte{1}, 32)})
	var we *Error
	if !errors.As(err, &we) {
		t.Fatalf("want *web.Error, got %T %v", err, err)
	}
	if we.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", we.StatusCode)
	}
}

func TestConfirmUsernameHash_Happy(t *testing.T) {
	hash := bytes.Repeat([]byte{0xAA}, 32)
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/username_hash/confirm" {
			t.Fatalf("path %s", r.URL.Path)
		}
		var body confirmUsernameRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.ZKProof == "" || body.EncryptedUsername == "" || body.UsernameHash == "" {
			t.Fatal("missing fields in body")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(confirmUsernameResponseBody{
			UsernameHash:       base64.RawURLEncoding.EncodeToString(hash),
			UsernameLinkHandle: "11111111-2222-3333-4444-555555555555",
		})
	})
	got, err := c.ConfirmUsernameHash(context.Background(), authCreds(), hash, []byte("proof"), []byte("link"))
	if err != nil {
		t.Fatalf("ConfirmUsernameHash: %v", err)
	}
	if !bytes.Equal(got.UsernameHash, hash) {
		t.Fatal("hash mismatch")
	}
	if got.UsernameLinkHandle != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("handle = %q", got.UsernameLinkHandle)
	}
}

func TestConfirmUsernameHash_ValidatesInput(t *testing.T) {
	c := New("http://unused", "test")
	hash := bytes.Repeat([]byte{1}, 32)
	if _, err := c.ConfirmUsernameHash(context.Background(), Credentials{}, hash, []byte("p"), []byte("l")); err == nil {
		t.Fatal("expected creds error")
	}
	if _, err := c.ConfirmUsernameHash(context.Background(), authCreds(), nil, []byte("p"), []byte("l")); err == nil {
		t.Fatal("expected hash error")
	}
	if _, err := c.ConfirmUsernameHash(context.Background(), authCreds(), hash, nil, []byte("l")); err == nil {
		t.Fatal("expected proof error")
	}
	if _, err := c.ConfirmUsernameHash(context.Background(), authCreds(), hash, []byte("p"), nil); err == nil {
		t.Fatal("expected link error")
	}
}

func TestDeleteUsernameHash(t *testing.T) {
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/accounts/username_hash" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.DeleteUsernameHash(context.Background(), authCreds()); err != nil {
		t.Fatalf("DeleteUsernameHash: %v", err)
	}
}

func TestSetUsernameLink_Happy(t *testing.T) {
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/username_link" {
			t.Fatalf("path %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var body setUsernameLinkRequestBody
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !body.KeepLinkHandle {
			t.Fatal("expected keepLinkHandle=true")
		}
		if body.UsernameLinkEncryptedValue == "" {
			t.Fatal("empty encrypted value")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(setUsernameLinkResponseBody{
			UsernameLinkHandle: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		})
	})
	got, err := c.SetUsernameLink(context.Background(), authCreds(), []byte("ciphertext"), true)
	if err != nil {
		t.Fatalf("SetUsernameLink: %v", err)
	}
	if got.UsernameLinkHandle == "" {
		t.Fatal("empty handle")
	}
}

func TestDeleteUsernameLink(t *testing.T) {
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/accounts/username_link" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.DeleteUsernameLink(context.Background(), authCreds()); err != nil {
		t.Fatalf("DeleteUsernameLink: %v", err)
	}
}

func TestLookupUsernameHash_UsesUrlSafeB64AndNoAuth(t *testing.T) {
	// Use a hash with bytes that distinguish url-safe from standard b64.
	hash := make([]byte, 32)
	for i := range hash {
		hash[i] = byte(0xFE - i)
	}
	expectedB64 := base64.RawURLEncoding.EncodeToString(hash)
	if strings.ContainsAny(expectedB64, "+/=") {
		t.Fatalf("url-safe b64 should not contain +/=, got %q", expectedB64)
	}
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("lookup must not send Authorization")
		}
		want := "/v1/accounts/username_hash/" + expectedB64
		if r.URL.Path != want {
			t.Fatalf("path = %q, want %q", r.URL.Path, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lookupUsernameHashResponseBody{UUID: "aci-uuid-here"})
	})
	got, err := c.LookupUsernameHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("LookupUsernameHash: %v", err)
	}
	if got.ACI != "aci-uuid-here" {
		t.Fatalf("ACI = %q", got.ACI)
	}
}

func TestLookupUsernameHash_404(t *testing.T) {
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := c.LookupUsernameHash(context.Background(), bytes.Repeat([]byte{1}, 32))
	var we *Error
	if !errors.As(err, &we) || we.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 *web.Error, got %T %v", err, err)
	}
}

func TestLookupUsernameLink_Happy(t *testing.T) {
	ciphertext := []byte("stuff")
	c := newUsernameServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("lookup must not send Authorization")
		}
		if r.URL.Path != "/v1/accounts/username_link/some-handle" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lookupUsernameLinkResponseBody{
			UsernameLinkEncryptedValue: base64.RawURLEncoding.EncodeToString(ciphertext),
		})
	})
	got, err := c.LookupUsernameLink(context.Background(), "some-handle")
	if err != nil {
		t.Fatalf("LookupUsernameLink: %v", err)
	}
	if !bytes.Equal(got.UsernameLinkEncryptedValue, ciphertext) {
		t.Fatal("ciphertext mismatch")
	}
}

func TestLookupUsernameLink_ValidatesHandle(t *testing.T) {
	c := New("http://unused", "test")
	if _, err := c.LookupUsernameLink(context.Background(), ""); err == nil {
		t.Fatal("expected error on empty handle")
	}
}
