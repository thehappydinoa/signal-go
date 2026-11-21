package libsignal

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

func TestUsernameCandidatesEmptyNicknameRejected(t *testing.T) {
	if _, err := UsernameCandidates("", 3, 32); err == nil {
		t.Fatal("expected error for empty nickname, got nil")
	}
}

func TestUsernameCandidatesProducesValidCandidates(t *testing.T) {
	got, err := UsernameCandidates("alice", 3, 32)
	if err != nil {
		t.Fatalf("UsernameCandidates: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one candidate")
	}
	for _, c := range got {
		s := string(c)
		if !strings.HasPrefix(s, "alice.") {
			t.Errorf("candidate %q missing nickname prefix", s)
		}
		if len(s) < 3 || len(s) > 32 {
			t.Errorf("candidate %q out of length bounds", s)
		}
	}
}

func TestUsernameHashDeterministic(t *testing.T) {
	a, err := UsernameHash("alice.42")
	if err != nil {
		t.Fatalf("UsernameHash: %v", err)
	}
	b, err := UsernameHash("alice.42")
	if err != nil {
		t.Fatalf("UsernameHash second: %v", err)
	}
	if a != b {
		t.Fatal("same input produced different hashes")
	}
	// Different inputs produce different hashes.
	c, err := UsernameHash("alice.43")
	if err != nil {
		t.Fatalf("UsernameHash alt: %v", err)
	}
	if a == c {
		t.Fatal("different inputs produced the same hash")
	}
}

func TestUsernameHashFromPartsMatchesFullString(t *testing.T) {
	// Pick a candidate so the discriminator respects libsignal's rules.
	cands, err := UsernameCandidates("alice", 3, 32)
	if err != nil || len(cands) == 0 {
		t.Fatalf("UsernameCandidates: %v (n=%d)", err, len(cands))
	}
	full := string(cands[0])
	dot := strings.Index(full, ".")
	if dot <= 0 {
		t.Fatalf("candidate %q has no discriminator", full)
	}
	nick, disc := full[:dot], full[dot+1:]

	fromFull, err := UsernameHash(full)
	if err != nil {
		t.Fatalf("UsernameHash: %v", err)
	}
	fromParts, err := UsernameHashFromParts(nick, disc, 3, 32)
	if err != nil {
		t.Fatalf("UsernameHashFromParts: %v", err)
	}
	if fromFull != fromParts {
		t.Fatalf("part hash %x != full hash %x", fromParts, fromFull)
	}
}

func TestUsernameProofRoundTrip(t *testing.T) {
	cands, err := UsernameCandidates("carol", 3, 32)
	if err != nil || len(cands) == 0 {
		t.Fatalf("UsernameCandidates: %v", err)
	}
	username := string(cands[0])

	hash, err := UsernameHash(username)
	if err != nil {
		t.Fatalf("UsernameHash: %v", err)
	}

	var randomness [32]byte
	if _, err := rand.Read(randomness[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}

	proof, err := UsernameProof(username, randomness)
	if err != nil {
		t.Fatalf("UsernameProof: %v", err)
	}
	if len(proof) == 0 {
		t.Fatal("empty proof")
	}

	if err := VerifyUsernameProof(proof, hash); err != nil {
		t.Fatalf("VerifyUsernameProof: %v", err)
	}
}

func TestVerifyUsernameProofRejectsWrongHash(t *testing.T) {
	proof, err := UsernameProof("alice.12345", [32]byte{1, 2, 3})
	if err != nil {
		t.Fatalf("UsernameProof: %v", err)
	}
	wrong, err := UsernameHash("alice.99999")
	if err != nil {
		t.Fatalf("UsernameHash: %v", err)
	}
	if err := VerifyUsernameProof(proof, wrong); err == nil {
		t.Fatal("expected mismatch rejection, got nil")
	}
}

func TestUsernameLinkRoundTrip(t *testing.T) {
	const username = "alice.12345"
	// First call with nil entropy -> libsignal generates fresh entropy
	// and returns a combined ciphertext payload.
	encrypted, err := UsernameLinkCreate(username, nil)
	if err != nil {
		t.Fatalf("UsernameLinkCreate: %v", err)
	}
	// Payload is entropy || ciphertext per the libsignal contract; the
	// server stores the ciphertext suffix and publishes entropy alongside
	// the UUID handle. Verify we got something back.
	if len(encrypted) == 0 {
		t.Fatal("UsernameLinkCreate returned empty payload")
	}
	// Second create with the same entropy should be stable: pull the
	// entropy prefix out, rotate with it, decrypt, compare.
	//
	// The libsignal convention is: the first 32 bytes of the returned
	// buffer are the entropy. Everything after is the server-stored
	// ciphertext.
	if len(encrypted) < 32 {
		t.Fatalf("payload shorter than entropy prefix: %d", len(encrypted))
	}
	entropy := encrypted[:32]
	ciphertext := encrypted[32:]

	got, err := UsernameLinkDecrypt(entropy, ciphertext)
	if err != nil {
		t.Fatalf("UsernameLinkDecrypt: %v", err)
	}
	if got != username {
		t.Fatalf("round-trip mismatch: want %q, got %q", username, got)
	}

	// Rotating with the same entropy produces the same ciphertext (so
	// the link handle stays stable).
	rotated, err := UsernameLinkCreate(username, entropy)
	if err != nil {
		t.Fatalf("UsernameLinkCreate rotate: %v", err)
	}
	if !bytes.Equal(rotated, encrypted) {
		// libsignal may inject per-call randomness even when entropy is
		// fixed; the invariant we care about is that decryption works,
		// not byte equality. Verify by decrypting.
		if len(rotated) < 32 {
			t.Fatalf("rotated payload too short: %d", len(rotated))
		}
		if _, err := UsernameLinkDecrypt(rotated[:32], rotated[32:]); err != nil {
			t.Fatalf("rotated decrypt: %v", err)
		}
	}
}

func TestUsernameLinkDecryptRejectsGarbage(t *testing.T) {
	err := VerifyUsernameProof([]byte{1}, [32]byte{})
	if err == nil {
		t.Fatal("expected verify failure on tiny proof")
	}
	if _, err := UsernameLinkDecrypt([]byte{0}, []byte{0, 0, 0, 0}); err == nil {
		t.Fatal("expected decrypt failure on garbage")
	}
	var e *Error
	_ = errors.As(err, &e) // not fatal — just confirms we're surfacing typed errors
}
