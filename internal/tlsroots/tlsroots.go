// Package tlsroots configures TLS trust for Signal service endpoints.
//
// Production hosts under *.signal.org use Signal's private TLS root (vendored
// from Signal-iOS as signal-messenger.cer), not public WebPKI CAs — see
// https://signal.org/blog/certifiably-fine/ and [ApplyRootCAs].
//
// A blank import of golang.org/x/crypto/x509roots/fallback still registers
// Mozilla NSS roots for environments where the OS trust store is empty
// (cgo Windows builds dialing non-Signal hosts).
//
// Import this package once per process (for example from [internal/ws] and
// [internal/web]); Go runs package init only once.
package tlsroots

import _ "golang.org/x/crypto/x509roots/fallback" // register embedded roots
