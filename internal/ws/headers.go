package ws

import "net/http"

// HeaderPairs flattens h into the "Name: value" strings Signal's
// WebSocketRequestMessage.headers field expects.
func HeaderPairs(h http.Header) []string {
	if len(h) == 0 {
		return nil
	}
	out := make([]string, 0, len(h))
	for k, vs := range h {
		for _, v := range vs {
			out = append(out, k+": "+v)
		}
	}
	return out
}
