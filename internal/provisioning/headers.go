package provisioning

import "net/http"

// newSignalHeaders builds the canonical HTTP headers Signal expects on a
// provisioning upgrade.
//
// Real Signal clients also send X-Signal-Receive-Stories etc., but the
// provisioning endpoint accepts the bare minimum.
func newSignalHeaders(userAgent string) http.Header {
	h := http.Header{}
	h.Set("X-Signal-Agent", userAgent)
	h.Set("User-Agent", userAgent)
	return h
}
