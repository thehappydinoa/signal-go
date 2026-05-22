package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchGroupAuthCredentials(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(GroupAuthCredentialsResponse{
			Credentials: []GroupAuthCredential{{
				Credential:     []byte{1, 2, 3},
				RedemptionTime: CurrentDaySeconds(time.Unix(1_700_000_000, 0)),
			}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	creds := Credentials{Username: "aci.1", Password: "pw"}
	day := CurrentDaySeconds(time.Unix(1_700_000_000, 0))
	resp, err := c.FetchGroupAuthCredentials(context.Background(), creds, day)
	if err != nil {
		t.Fatalf("FetchGroupAuthCredentials: %v", err)
	}
	if gotPath != "/v1/certificate/auth/group" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("auth = %q", gotAuth)
	}
	if len(resp.Credentials) != 1 || len(resp.Credentials[0].Credential) != 3 {
		t.Fatalf("resp: %+v", resp)
	}
}

func TestFetchGroupState(t *testing.T) {
	var gotAuth, gotAccept string
	body := []byte{0x0a, 0x01, 0x08}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		if r.URL.Path != "/v2/groups/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	raw, err := c.FetchGroupState(context.Background(), "Basic abc")
	if err != nil {
		t.Fatalf("FetchGroupState: %v", err)
	}
	if gotAuth != "Basic abc" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotAccept != "application/x-protobuf" {
		t.Fatalf("accept = %q", gotAccept)
	}
	if string(raw) != string(body) {
		t.Fatalf("body = %v", raw)
	}
}

func TestCurrentDaySeconds(t *testing.T) {
	ts := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
	day := CurrentDaySeconds(ts)
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC).Unix()
	if day != want {
		t.Fatalf("day = %d, want %d", day, want)
	}
}
