// Package web is a thin HTTP client for Signal's REST API
// (chat.signal.org).
//
// It deliberately keeps the surface minimal: a request builder, JSON
// encode/decode, basic auth, and an error type that surfaces the status
// code + response body so higher layers can map server errors to typed
// outcomes.
//
// Authentication is HTTP Basic with the credentials Signal expects for
// each endpoint:
//
//   - /v1/devices/link        Basic(provisioningCode, password); production
//                             requires the provisioning websocket ([LinkDeviceWebSocket])
//   - /v1/devices/...         Basic("{ACI}.{deviceID}", password)
//   - /v2/keys, /v1/messages  same as above
//
// The package does not own any state — Account/Store handle that.
package web
