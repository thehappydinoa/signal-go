package fsstore

import (
	"os"
	"runtime"
	"testing"
)

// AssertFileMode0600 checks that path is owner-read/write only. Skipped on
// Windows: NTFS does not reflect Unix permission bits from os.Chmod, so
// production still calls Chmod(0o600) but Mode().Perm() stays 0666 there.
// Linux and macOS CI continue to enforce the contract from ADR 0012.
func AssertFileMode0600(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("%s mode = %v, want 0600", path, info.Mode().Perm())
	}
}
