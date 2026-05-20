// Package ws speaks Signal's request/response envelope (WebSocketMessage)
// over a [github.com/coder/websocket] connection.
//
// Signal's server protocol is layered:
//
//  1. An RFC-6455 websocket carried over TLS (handled by coder/websocket).
//  2. Each ws binary frame is a protobuf WebSocketMessage that wraps either
//     a WebSocketRequestMessage or WebSocketResponseMessage keyed by an
//     opaque request id. This gives Signal a request/response pattern on
//     top of a long-lived ws.
//
// This package owns layer 2. Layer 1 is provided by coder/websocket.
//
// Both the unauthenticated provisioning ws and the authenticated chat ws
// share these primitives; the only differences are the URL path and the
// HTTP headers passed at dial time. See [Dial].
package ws
