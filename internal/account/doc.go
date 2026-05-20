// Package account models a registered linked-device Signal account: the
// identifiers (ACI/PNI/device), credentials (HTTP Basic password),
// long-term identity keys, and the most recently issued signed / Kyber
// prekeys for both namespaces.
//
// An Account is the durable post-registration state. Per-recipient session
// material (Double Ratchet state, sender keys) lives in separate stores
// owned by the higher layers.
package account
