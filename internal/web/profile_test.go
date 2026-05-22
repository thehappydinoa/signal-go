package web

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchVersionedProfile(t *testing.T) {
	const aci = "9d0652a3-dcc3-4d11-975f-74d61598733f"
	const version = "abc123version"
	uak := bytes16(0x24)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		wantPath := "/v1/profile/" + aci + "/" + version
		if r.URL.Path != wantPath {
			t.Errorf("path = %s, want %s", r.URL.Path, wantPath)
		}
		gotKey := r.Header.Get("Unidentified-Access-Key")
		if gotKey != base64.StdEncoding.EncodeToString(uak) {
			t.Errorf("UAK header = %q", gotKey)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"","about":"","avatar":"/a/b"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	resp, err := c.FetchVersionedProfile(context.Background(), aci, version, uak)
	if err != nil {
		t.Fatalf("FetchVersionedProfile: %v", err)
	}
	if resp.Avatar != "/a/b" {
		t.Errorf("avatar = %q", resp.Avatar)
	}
}

func bytes16(v byte) []byte {
	b := make([]byte, 16)
	for i := range b {
		b[i] = v
	}
	return b
}
