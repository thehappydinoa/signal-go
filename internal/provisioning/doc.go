// Package provisioning implements the secondary-device half of Signal's QR
// link flow.
//
// # Overview
//
// To link as a Signal secondary device the flow is:
//
//  1. Generate an ephemeral Curve25519 keypair (the "provisioning keypair").
//  2. Open an unauthenticated websocket to
//     wss://chat.signal.org/v1/websocket/provisioning/.
//  3. The server sends a REQUEST `PUT /v1/address` carrying a
//     [ProvisioningAddress]; we ack with HTTP 200.
//  4. Compose a sgnl://linkdevice URL containing the address and our
//     ephemeral public key. The user scans this with their primary device.
//  5. After the user approves the link, the server sends a REQUEST
//     `PUT /v1/message` carrying a [ProvisionEnvelope]. We ack with 200,
//     decrypt the envelope using our ephemeral private key, and obtain a
//     [ProvisionMessage] containing the account's ACI/PNI identity keys,
//     UUIDs, number, and provisioning code.
//  6. Generate prekeys (ACI + PNI; signed + last-resort Kyber + 100 one-time
//     + 100 one-time Kyber) and `PUT /v1/devices/link` with credentials.
//
// This Phase-1 implementation covers steps 1-4 plus receiving the envelope
// in step 5. Decryption and step 6 are Phase 2.
//
// [ProvisioningAddress]: ../proto/gen/provisioningpb
// [ProvisionEnvelope]: ../proto/gen/provisioningpb
// [ProvisionMessage]: ../proto/gen/provisioningpb
package provisioning
