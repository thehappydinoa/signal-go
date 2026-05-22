package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchDirectoryAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/directory/auth" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"dir-user","password":"dir-pass"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	got, err := c.FetchDirectoryAuth(context.Background(), Credentials{Username: "aci.1", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "dir-user" || got.Password != "dir-pass" {
		t.Fatalf("got %+v", got)
	}
}
