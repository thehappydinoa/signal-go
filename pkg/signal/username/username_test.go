package username_test

import (
	"strings"
	"testing"

	"github.com/thehappydinoa/signal-go/pkg/signal/username"
)

func TestCandidatesDefaults(t *testing.T) {
	cs, err := username.Candidates("alice", 0, 0)
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(cs) == 0 {
		t.Fatal("no candidates returned")
	}
	for _, c := range cs {
		if !strings.HasPrefix(string(c), "alice.") {
			t.Errorf("unexpected candidate %q", c)
		}
	}
}

func TestCandidatesRejectsEmpty(t *testing.T) {
	if _, err := username.Candidates("", 0, 0); err == nil {
		t.Fatal("expected error for empty nickname")
	}
}

func TestProofVerifyRoundTrip(t *testing.T) {
	cs, err := username.Candidates("bob", 0, 0)
	if err != nil || len(cs) == 0 {
		t.Fatalf("Candidates: %v (n=%d)", err, len(cs))
	}
	c := cs[0]

	hash, err := c.Hash()
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	proof, err := c.Proof(nil) // auto-randomness
	if err != nil {
		t.Fatalf("Proof: %v", err)
	}
	if err := username.Verify(proof, hash); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// Cross-check: a different candidate's hash should reject this proof.
	cs2, err := username.Candidates("alice", 0, 0)
	if err != nil || len(cs2) == 0 {
		t.Fatalf("Candidates alt: %v", err)
	}
	wrongHash, err := cs2[0].Hash()
	if err != nil {
		t.Fatalf("Hash alt: %v", err)
	}
	if err := username.Verify(proof, wrongHash); err == nil {
		t.Fatal("expected cross-identity rejection")
	}
}

func TestProofRejectsBadRandomnessLen(t *testing.T) {
	c := username.Candidate("alice.12345")
	if _, err := c.Proof([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected rejection for 3-byte randomness")
	}
}

func TestLinkRoundTrip(t *testing.T) {
	const name = "carol.98765"
	link, err := username.NewLink(name)
	if err != nil {
		t.Fatalf("NewLink: %v", err)
	}
	if len(link.Entropy) != 32 {
		t.Fatalf("entropy length %d, want 32", len(link.Entropy))
	}
	if len(link.Ciphertext) == 0 {
		t.Fatal("empty ciphertext")
	}
	got, err := username.DecryptLink(link.Entropy, link.Ciphertext)
	if err != nil {
		t.Fatalf("DecryptLink: %v", err)
	}
	if string(got) != name {
		t.Fatalf("round-trip mismatch: want %q, got %q", name, got)
	}
}

func TestLinkRotateKeepsEntropy(t *testing.T) {
	link, err := username.NewLink("carol.98765")
	if err != nil {
		t.Fatalf("NewLink: %v", err)
	}
	rotated, err := link.RotateLink("carol.98765")
	if err != nil {
		t.Fatalf("RotateLink: %v", err)
	}
	if string(rotated.Entropy) != string(link.Entropy) {
		t.Fatal("RotateLink changed entropy — the shareable URL would change")
	}
	// Decrypt should still round-trip.
	got, err := username.DecryptLink(rotated.Entropy, rotated.Ciphertext)
	if err != nil {
		t.Fatalf("DecryptLink after rotate: %v", err)
	}
	if string(got) != "carol.98765" {
		t.Fatalf("rotated decrypt mismatch: %q", got)
	}
}

func TestDecryptLinkRejectsGarbage(t *testing.T) {
	if _, err := username.DecryptLink(make([]byte, 32), []byte{0, 0, 0, 0}); err == nil {
		t.Fatal("expected error on garbage ciphertext")
	}
}
