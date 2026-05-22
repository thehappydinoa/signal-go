package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/proto"

	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
)

func TestFetchStorageAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/storage/auth" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"u","password":"p"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	got, err := c.FetchStorageAuth(context.Background(), Credentials{Username: "aci.1", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "u" || got.Password != "p" {
		t.Fatalf("got %+v", got)
	}
}

func TestFetchStorageManifestStatuses(t *testing.T) {
	manifest := &storagepb.StorageManifest{Version: 3, Value: []byte{1, 2, 3}}
	raw, err := proto.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/storage/manifest/version/2" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(raw)
		}))
		defer srv.Close()
		c := New(srv.URL, "test")
		result, err := c.FetchStorageManifest(context.Background(), Credentials{Username: "u", Password: "p"}, 2)
		if err != nil {
			t.Fatal(err)
		}
		if result.Unchanged || result.Missing || result.Manifest.GetVersion() != 3 {
			t.Fatalf("got %+v", result)
		}
	})

	t.Run("unchanged", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()
		c := New(srv.URL, "test")
		result, err := c.FetchStorageManifest(context.Background(), Credentials{Username: "u", Password: "p"}, 5)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Unchanged {
			t.Fatalf("got %+v", result)
		}
	})

	t.Run("missing", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		c := New(srv.URL, "test")
		result, err := c.FetchStorageManifest(context.Background(), Credentials{Username: "u", Password: "p"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Missing {
			t.Fatalf("got %+v", result)
		}
	})
}

func TestReadStorageRecords(t *testing.T) {
	var gotMethod, gotContentType string
	items := &storagepb.StorageItems{
		Items: []*storagepb.StorageItem{{Key: []byte{1}, Value: []byte{2}}},
	}
	respBody, err := proto.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}))
	defer srv.Close()

	c := New(srv.URL, "test")
	got, err := c.ReadStorageRecords(context.Background(), Credentials{Username: "u", Password: "p"}, [][]byte{{9}})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotContentType != "application/x-protobuf" {
		t.Fatalf("method=%q content-type=%q", gotMethod, gotContentType)
	}
	if len(got.GetItems()) != 1 {
		t.Fatalf("items = %+v", got)
	}
}
