package tlsroots

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"sync"
)

// signalMessengerRootSHA256 is the SHA-256 fingerprint of the production
// Signal Messenger, LLC root shipped in Signal-iOS as
// SignalServiceKit/Resources/Certificates/signal-messenger.cer (checked
// 2026-05-22 against chat.signal.org).
const signalMessengerRootSHA256 = "ddb0f92bb95c8d6fd202ea6e8cc5ccd182b544f8cd696f47d580659ddc9df65a"

// signalMessengerRootDER is the DER-encoded Signal production TLS root.
// Source: https://github.com/signalapp/Signal-iOS (signal-messenger.cer).
//
//go:embed certs/signal-messenger.cer
var signalMessengerRootDER []byte

var (
	signalPool     *x509.CertPool
	signalPoolOnce sync.Once
	signalPoolErr  error
)

// IsSignalHost reports whether host should use [SignalRootCAs] for TLS
// verification. Matches *.signal.org and signal.org (case-insensitive).
func IsSignalHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return false
	}
	if host == "signal.org" {
		return true
	}
	return strings.HasSuffix(host, ".signal.org")
}

// SignalRootCAs returns a pool containing only Signal's private production
// TLS root. Used for chat.signal.org and other *.signal.org endpoints;
// public WebPKI roots are intentionally not included (see Signal's
// "Certifiably Fine" blog post).
func SignalRootCAs() (*x509.CertPool, error) {
	signalPoolOnce.Do(func() {
		cert, err := x509.ParseCertificate(signalMessengerRootDER)
		if err != nil {
			signalPoolErr = err
			return
		}
		sum := sha256.Sum256(cert.Raw)
		got := hex.EncodeToString(sum[:])
		if got != signalMessengerRootSHA256 {
			signalPoolErr = errFingerprintMismatch
			return
		}
		signalPool = x509.NewCertPool()
		signalPool.AddCert(cert)
	})
	return signalPool, signalPoolErr
}

// ForHost returns [SignalRootCAs] when host is a Signal service hostname,
// otherwise nil (caller keeps system / fallback roots).
func ForHost(host string) (*x509.CertPool, error) {
	if !IsSignalHost(host) {
		return nil, nil
	}
	return SignalRootCAs()
}

// ApplyRootCAs sets cfg.RootCAs to Signal's pool when host matches and
// cfg.RootCAs is nil. Non-Signal hosts are unchanged.
func ApplyRootCAs(cfg *tls.Config, host string) error {
	if cfg == nil || cfg.RootCAs != nil {
		return nil
	}
	pool, err := ForHost(host)
	if err != nil {
		return err
	}
	if pool != nil {
		cfg.RootCAs = pool
	}
	return nil
}

// Hostname extracts the host part from hostport (from net/url Host).
func Hostname(hostport string) string {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return h
}

var errFingerprintMismatch = errors.New("tlsroots: embedded Signal root fingerprint mismatch")
