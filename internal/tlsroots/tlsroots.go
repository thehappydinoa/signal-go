// Package tlsroots registers Mozilla NSS-derived fallback X.509 roots via
// [crypto/x509.SetFallbackRoots]. Go binaries linked with cgo on Windows
// (and some container images) often cannot load the OS trust store, so
// [tls.Config] with RootCAs nil would otherwise fail with "certificate signed
// by unknown authority" against chat.signal.org.
//
// Import this package once per process (for example from [internal/ws] and
// [internal/web]); Go runs package init only once.
package tlsroots

import _ "golang.org/x/crypto/x509roots/fallback" // register embedded roots
