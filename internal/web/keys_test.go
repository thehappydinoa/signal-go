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

func TestUploadPreKeysSendsExpectedRequest(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth string
	var gotBody UploadPreKeysRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := New(srv.URL, "test")
	creds := Credentials{Username: "aci-uuid.1", Password: "pw"}
	req := UploadPreKeysRequest{
		IdentityKey: base64.StdEncoding.EncodeToString([]byte("identity")),
		PreKeys: []ECPreKey{
			{KeyID: 2, PublicKey: "AA"},
			{KeyID: 3, PublicKey: "BB"},
		},
		PqPreKeys: []KEMPreKey{
			{KeyID: 2, PublicKey: "CC", Signature: "DD"},
		},
	}
	if err := c.UploadPreKeys(context.Background(), creds, IdentityACI, req); err != nil {
		t.Fatalf("UploadPreKeys: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/v2/keys" {
		t.Errorf("method/path: %s %s", gotMethod, gotPath)
	}
	if gotQuery != "identity=aci" {
		t.Errorf("query = %q", gotQuery)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("aci-uuid.1:pw"))
	if gotAuth != wantAuth {
		t.Errorf("auth = %q", gotAuth)
	}
	if len(gotBody.PreKeys) != 2 || gotBody.PreKeys[1].KeyID != 3 {
		t.Errorf("PreKeys = %+v", gotBody.PreKeys)
	}
	if len(gotBody.PqPreKeys) != 1 || gotBody.PqPreKeys[0].Signature != "DD" {
		t.Errorf("PqPreKeys = %+v", gotBody.PqPreKeys)
	}
}

func TestUploadPreKeysValidatesInputs(t *testing.T) {
	c := New("https://example.com", "test")
	cases := []struct {
		name string
		call func() error
	}{
		{"missing creds", func() error {
			return c.UploadPreKeys(context.Background(), Credentials{}, IdentityACI, UploadPreKeysRequest{IdentityKey: "x"})
		}},
		{"bad identity", func() error {
			return c.UploadPreKeys(context.Background(), Credentials{Username: "u", Password: "p"}, "bogus", UploadPreKeysRequest{IdentityKey: "x"})
		}},
		{"missing identity key", func() error {
			return c.UploadPreKeys(context.Background(), Credentials{Username: "u", Password: "p"}, IdentityACI, UploadPreKeysRequest{})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestUploadPreKeysSurfacesServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(srv.URL, "test")
	err := c.UploadPreKeys(context.Background(), Credentials{Username: "u", Password: "p"}, IdentityACI, UploadPreKeysRequest{IdentityKey: "x"})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v", err)
	}
}
