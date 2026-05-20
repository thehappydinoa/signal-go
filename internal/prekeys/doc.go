// Package prekeys assembles the prekey bundles a Signal device must publish
// when it links or refreshes its key material.
//
// Two flavours exist, both for ACI and PNI namespaces:
//
//   - Curve25519 prekeys (X3DH):
//     a single rotating signed prekey + a batch of one-time prekeys.
//   - Kyber / ML-KEM prekeys (PQXDH, mandatory since March 2026):
//     a single rotating "last resort" prekey + a batch of one-time prekeys.
//
// All signatures are XEdDSA over the prekey's public bytes, signed by the
// owning identity (ACI or PNI) private key. Both ECDH and signing flow
// through libsignal.
//
// IDs are 14-bit unsigned ints in Signal's wire format (range 1..0x3FFE).
// Callers are responsible for tracking the next ID to use; helper
// generators in this package only take a starting id and bump from there.
package prekeys
