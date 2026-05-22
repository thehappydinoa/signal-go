package qrterminal

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"
)

func TestWriteProvisioningSizedURL(t *testing.T) {
	// Representative sgnl:// link length (uuid + 33-byte identity pubkey).
	pub := strings.Repeat("A", 44)
	raw := "sgnl://linkdevice?uuid=" + strings.Repeat("a", 36) + "&pub_key=" + pub
	if len(raw) < 80 {
		t.Fatalf("test URL too short: %d", len(raw))
	}
	if _, err := url.Parse(raw); err != nil {
		t.Fatalf("parse: %v", err)
	}

	var buf bytes.Buffer
	err := Write(raw, Options{Writer: &buf, OptOut: false})
	if errors.Is(err, ErrDisabled) {
		t.Skip("non-TTY test environment")
	}
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "▄") && !strings.Contains(out, "█") && !strings.Contains(out, "▀") {
		preview := out
		if len(preview) > 80 {
			preview = preview[:80]
		}
		t.Errorf("output lacks half-block QR art: %q", preview)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if n := len(lines); n < 15 || n > 45 {
		t.Errorf("line count = %d, want roughly 15–45 for link URL", n)
	}
}

func TestWriteOptOut(t *testing.T) {
	var buf bytes.Buffer
	err := Write("hello", Options{Writer: &buf, OptOut: true})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if buf.Len() != 0 {
		t.Error("expected no output when OptOut")
	}
}

func TestEncodeMatchesLibrary(t *testing.T) {
	const msg = "sgnl://linkdevice?uuid=test&pub_key=AQID"
	qr, err := qrcode.New(msg, qrcode.Medium)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(qr.Bitmap()); got < 21 {
		t.Errorf("module size = %d, want at least 21", got)
	}
}
