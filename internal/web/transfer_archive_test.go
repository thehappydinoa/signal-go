package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/internal/web"
)

func TestFetchTransferArchivePollAndSuccess(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices/transfer_archive" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got == "" {
			t.Errorf("missing Authorization header")
		}
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cdn": 3,
			"key": "transfer-key",
		})
	}))
	defer srv.Close()

	webc := web.New(srv.URL, "test")
	creds := web.Credentials{Username: "aci.device", Password: "secret"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got, err := webc.FetchTransferArchive(ctx, creds, 30*time.Second)
	if err != nil {
		t.Fatalf("FetchTransferArchive: %v", err)
	}
	if got.Archive == nil || got.Archive.CDNKey != "transfer-key" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if calls.Load() < 2 {
		t.Fatalf("expected at least 2 poll attempts, got %d", calls.Load())
	}
}

func TestFetchTransferArchiveContinueWithoutUpload(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "CONTINUE_WITHOUT_UPLOAD"})
	}))
	defer srv.Close()

	webc := web.New(srv.URL, "test")
	got, err := webc.FetchTransferArchive(context.Background(), web.Credentials{
		Username: "u", Password: "p",
	}, time.Minute)
	if err != nil {
		t.Fatalf("FetchTransferArchive: %v", err)
	}
	if got.Error != web.TransferArchiveContinueWithoutUpload {
		t.Fatalf("got error %q", got.Error)
	}
}
