package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNextGroupLogRevision(t *testing.T) {
	got, err := NextGroupLogRevision("versions 5-68/100")
	if err != nil {
		t.Fatal(err)
	}
	if got != 69 {
		t.Fatalf("got %d, want 69", got)
	}
}

func TestNextGroupLogRevisionRejectsBadFormat(t *testing.T) {
	if _, err := NextGroupLogRevision("bytes 0-1/2"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchGroupLogs(t *testing.T) {
	var gotPath, gotCached, gotAccept string
	body := []byte{0x0a, 0x00}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCached = r.Header.Get("Cached-Send-Endorsements")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Range", "versions 0-63/128")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	page, err := c.FetchGroupLogs(context.Background(), "Basic abc", 10, GroupLogsOptions{
		CachedSendEndorsementsExpiration: 1_800_000_000,
		IncludeLastState:                 true,
	})
	if err != nil {
		t.Fatalf("FetchGroupLogs: %v", err)
	}
	if gotPath != "/v2/groups/logs/10" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotCached != "1800000000" {
		t.Fatalf("cached = %q", gotCached)
	}
	if gotAccept != "application/x-protobuf" {
		t.Fatalf("accept = %q", gotAccept)
	}
	if page.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d", page.StatusCode)
	}
	if page.ContentRange != "versions 0-63/128" {
		t.Fatalf("content-range = %q", page.ContentRange)
	}
	if string(page.Body) != string(body) {
		t.Fatalf("body mismatch")
	}
}
