// Package chat provides an authenticated WebSocket connection to Signal's
// chat service (wss://chat.signal.org/v1/websocket/). It wraps
// [internal/ws.Client] with automatic reconnection, exponential backoff,
// and envelope-level dispatch.
//
// The connection authenticates via HTTP Basic using the linked device's
// "{ACI}.{deviceId}:{password}" credentials. Inbound envelopes arrive as
// server-initiated WebSocketRequestMessages on PUT /api/v1/message; the
// Connection acknowledges each one and forwards the raw envelope bytes to
// a caller-supplied handler.
//
// Reconnect uses capped exponential backoff with jitter:
// 1s, 2s, 4s, 8s, 16s, 30s, 60s max. Each reconnect replays the auth
// handshake. The dispatch loop treats reconnects as transparent — callers
// see a continuous stream of envelopes.
//
// See [ADR 0010] for the design, and [docs/diagrams/receive-pipeline.md]
// for the data-flow diagram.
//
// [ADR 0010]: ../../docs/adr/0010-phase-3-receive.md
package chat
