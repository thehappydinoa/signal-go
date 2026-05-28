package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutGroup(t *testing.T) {
	var gotMethod, gotAuth, gotContentType string
	body := []byte{0x0a, 0x02, 0x08, 0x00}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if r.URL.Path != "/v2/groups/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	raw, err := c.PutGroup(context.Background(), "Basic abc", []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("PutGroup: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotAuth != "Basic abc" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotContentType != "application/x-protobuf" {
		t.Fatalf("content-type = %q", gotContentType)
	}
	if string(raw) != string(body) {
		t.Fatalf("body = %v", raw)
	}
}
