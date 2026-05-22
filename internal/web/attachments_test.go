package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAndUploadAttachmentCDN3(t *testing.T) {
	var uploaded []byte
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if r.Header.Get("Tus-Resumable") != "1.0.0" {
				t.Errorf("missing tus header")
			}
			body, _ := io.ReadAll(r.Body)
			uploaded = body
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			if r.URL.Path != "/attachments/abc123" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(uploaded)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer cdn.Close()

	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4/attachments/form/upload" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cdn":                  3,
			"key":                  "abc123",
			"headers":              map[string]string{"Authorization": "Bearer x"},
			"signedUploadLocation": cdn.URL,
		})
	}))
	defer chat.Close()

	webc := New(chat.URL, "test")
	webc.CDNHosts = map[uint32]string{3: cdn.URL}
	creds := Credentials{Username: "u", Password: "p"}
	form, err := webc.FetchAttachmentUploadForm(context.Background(), creds, 5)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := []byte{1, 2, 3, 4, 5}
	if err := webc.UploadAttachment(context.Background(), form, ciphertext); err != nil {
		t.Fatal(err)
	}
	got, err := webc.DownloadAttachmentCDN(context.Background(), 3, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(ciphertext) {
		t.Fatalf("download = %v", got)
	}
}

func TestCDNAttachmentURL(t *testing.T) {
	u, err := CDNAttachmentURL(3, "foo/bar")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://cdn3.signal.org/attachments/foo%2Fbar" {
		t.Fatalf("url = %q", u)
	}
}
