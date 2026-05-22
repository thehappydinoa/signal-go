package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchGroupJoinInfo(t *testing.T) {
	body := []byte{0x0a, 0x02, 0x08, 0x01}
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	raw, err := c.FetchGroupJoinInfo(context.Background(), "Basic abc", "cGFzcw==")
	if err != nil {
		t.Fatalf("FetchGroupJoinInfo: %v", err)
	}
	if gotPath != "/v2/groups/join/cGFzcw==" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Basic abc" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if string(raw) != string(body) {
		t.Fatalf("body = %v", raw)
	}
}

func TestPatchGroupWithInvite(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte{0x0a, 0x00})
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	_, err := c.PatchGroupWithInvite(context.Background(), "Basic abc", "cGFzcw==", []byte{1, 2})
	if err != nil {
		t.Fatalf("PatchGroupWithInvite: %v", err)
	}
	if gotQuery != "inviteLinkPassword=cGFzcw%3D%3D" && gotQuery != "inviteLinkPassword=cGFzcw==" {
		t.Fatalf("query = %q", gotQuery)
	}
}
