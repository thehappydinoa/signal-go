// Package prekeymaint uploads one-time prekey batches (PUT /v2/keys) and
// tops up the local pool when counts run low after inbound decrypt. It
// sits outside [internal/prekeys] to avoid import cycles with
// [internal/account] and [internal/web].
package prekeymaint
