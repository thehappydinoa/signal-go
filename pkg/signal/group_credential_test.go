package signal

import "testing"

// TestInvalidateGroupAuthCache exercises the Phase-8 audit hook for the
// zkgroup auth credential cache. The eviction is a no-op on an empty
// cache, and clears every entry on a populated cache. The implementation
// is trivial; the test exists so the public guarantee documented on the
// method does not regress under refactors.
func TestInvalidateGroupAuthCache(t *testing.T) {
	c := &Client{}
	// Empty cache: must not panic and must remain empty.
	c.InvalidateGroupAuthCache()
	if c.groupAuthCreds != nil {
		t.Fatalf("groupAuthCreds = %v, want nil after invalidate", c.groupAuthCreds)
	}

	// Populate the cache directly and ensure it gets wiped.
	c.groupAuthCreds = map[int64][]byte{
		1000: {1, 2, 3},
		2000: {4, 5, 6},
	}
	c.InvalidateGroupAuthCache()
	if c.groupAuthCreds != nil {
		t.Fatalf("InvalidateGroupAuthCache left entries: %v", c.groupAuthCreds)
	}

	// A second invalidate on an already-cleared cache is still safe.
	c.InvalidateGroupAuthCache()
}
