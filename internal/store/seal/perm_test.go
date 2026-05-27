package seal

import (
	"os"
	"runtime"
	"testing"
)

// AssertFileMode0600 checks that path is owner-read/write only. Skipped on
// Windows where NTFS does not reflect Unix permission bits from os.Chmod.
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
