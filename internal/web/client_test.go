package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCredentialsHeader(t *testing.T) {
	if got := (Credentials{}).Header(); got != "" {
		t.Errorf("empty creds Header = %q, want empty", got)
	}
	got := Credentials{Username: "u", Password: "p"}.Header()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))
	if got != want {
		t.Errorf("Header = %q, want %q", got, want)
	}
}

func TestDoSendsAuthAndUserAgent(t *testing.T) {
	var gotAuth, gotUA, gotSigAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotSigAgent = r.Header.Get("X-Signal-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := New(srv.URL, "test-agent")
	err := c.Do(context.Background(), Request{
		Method:      http.MethodGet,
		Path:        "/v1/health",
		Credentials: Credentials{Username: "alice", Password: "s3cret"},
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotUA != "test-agent" || gotSigAgent != "test-agent" {
		t.Errorf("user-agent headers: ua=%q sig=%q", gotUA, gotSigAgent)
	}
}

func TestDoRoundtripsJSON(t *testing.T) {
	type req struct {
		Name string `json:"name"`
	}
	type resp struct {
		Greeting string `json:"greeting"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "want json", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var in req
		if err := json.Unmarshal(body, &in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(resp{Greeting: "hello " + in.Name})
	}))
	defer srv.Close()
	c := New(srv.URL, "test")
	var out resp
	err := c.Do(context.Background(), Request{
		Method: http.MethodPost,
		Path:   "/echo",
		Body:   req{Name: "world"},
		Out:    &out,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.Greeting != "hello world" {
		t.Errorf("Greeting = %q", out.Greeting)
	}
}

func TestDoSurfacesHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	c := New(srv.URL, "test")
	err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/x"})
	var herr *Error
	if !errors.As(err, &herr) {
		t.Fatalf("expected *Error, got %T (%v)", err, err)
	}
	if herr.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d", herr.StatusCode)
	}
	if !strings.Contains(string(herr.Body), "nope") {
		t.Errorf("body = %q", herr.Body)
	}
}

func TestDoHonoursContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()
	c := New(srv.URL, "test")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.Do(ctx, Request{Method: http.MethodGet, Path: "/slow"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestBuildURLValidation(t *testing.T) {
	c := New("https://example.com", "test")
	if _, err := c.buildURL("no-leading-slash", nil); err == nil {
		t.Error("expected error for path without leading /")
	}
}

func TestNewEnforcesTLSMinVersion(t *testing.T) {
	c := New("https://example.com", "test")
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", c.HTTPClient.Transport)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil; want explicit MinVersion=1.2")
	}
	if tr.TLSClientConfig.MinVersion != MinTLSVersion {
		t.Errorf("MinVersion = 0x%x, want 0x%x", tr.TLSClientConfig.MinVersion, MinTLSVersion)
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("default TLS config must verify certificates")
	}
}

func TestNewWithOptionsPanicsOnProdInsecure(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when InsecureSkipVerify is set against prod base URL")
		}
	}()
	_ = NewWithOptions(DefaultBaseURL, "test", Options{InsecureSkipVerify: true})
}
