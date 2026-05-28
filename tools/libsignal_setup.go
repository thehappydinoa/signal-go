//go:build ignore

// libsignal_setup.go downloads a pre-built libsignal_ffi.a for the current
// platform from the project's GitHub Releases, avoiding the need to install
// Rust and cargo.
//
// Usage (from the repo root):
//
//	go run tools/libsignal_setup.go
//
// Or via go generate (from internal/libsignal/):
//
//	go generate .
//
// If no pre-built artifact is available (e.g. a development commit between
// releases), the tool prints the fallback command and exits non-zero:
//
//	task libsignal   # requires Rust — see docs/guides/getting-started.md
package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const githubRepo = "thehappydinoa/signal-go"

func main() {
	root, err := moduleRoot()
	if err != nil {
		fatalf("cannot locate module root: %v", err)
	}

	version, err := libsignalVersion(filepath.Join(root, "scripts", "build-libsignal.sh"))
	if err != nil {
		fatalf("cannot read libsignal version: %v", err)
	}

	osName, arch, err := platformNames()
	if err != nil {
		fatalf("%v\n\nFallback: run 'task libsignal' (requires Rust+cargo+nasm).\nSee docs/guides/getting-started.md for prerequisites.", err)
	}

	libDir := filepath.Join(root, "internal", "libsignal", "lib")
	stampFile := filepath.Join(libDir, ".version")
	stampValue := fmt.Sprintf("%s-%s-%s", version, osName, arch)

	// Exit early if the artifact is already present and stamped.
	if os.Getenv("FORCE") != "1" {
		if b, err := os.ReadFile(stampFile); err == nil {
			if strings.TrimSpace(string(b)) == stampValue {
				fmt.Fprintf(os.Stderr, "libsignal_ffi.a already present for %s on %s/%s\n", version, osName, arch)
				return
			}
		}
	}

	assetName := fmt.Sprintf("libsignal-ffi-%s-%s-%s.a", version, osName, arch)
	releaseTag := "libsignal-" + version
	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", githubRepo, releaseTag)
	assetURL := baseURL + "/" + assetName
	sha256URL := baseURL + "/" + assetName + ".sha256"

	fmt.Fprintf(os.Stderr, ">> downloading %s\n", assetName)
	data, err := downloadURL(assetURL)
	if err != nil {
		fatalf("download failed: %v\n\nNo pre-built artifact is available yet for %s/%s at %s.\nFallback: run 'task libsignal' (requires Rust+cargo+nasm).\nSee docs/guides/getting-started.md for prerequisites.", osName, arch, assetURL, err)
	}

	// Verify SHA256 when the checksum file is reachable.
	if sha256Data, err := downloadURL(sha256URL); err == nil {
		expected := strings.Fields(string(sha256Data))[0]
		sum := sha256.Sum256(data)
		actual := fmt.Sprintf("%x", sum)
		if actual != expected {
			fatalf("SHA256 mismatch for %s: expected %s, got %s", assetName, expected, actual)
		}
		fmt.Fprintf(os.Stderr, ">> SHA256 verified: %s\n", actual)
	} else {
		fmt.Fprintf(os.Stderr, ">> warning: SHA256 file unavailable; skipping checksum verification\n")
	}

	if err := os.MkdirAll(libDir, 0o755); err != nil {
		fatalf("cannot create lib dir: %v", err)
	}
	destPath := filepath.Join(libDir, "libsignal_ffi.a")
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		fatalf("cannot write %s: %v", destPath, err)
	}
	if err := os.WriteFile(stampFile, []byte(stampValue), 0o644); err != nil {
		fatalf("cannot write stamp: %v", err)
	}

	fi, _ := os.Stat(destPath)
	size := int64(0)
	if fi != nil {
		size = fi.Size()
	}
	fmt.Fprintf(os.Stderr, ">> installed %s (%d bytes)\n", destPath, size)
}

// moduleRoot walks up from the current working directory to find the directory
// containing go.mod (the module root).
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found (started from %s)", dir)
		}
		dir = parent
	}
}

// libsignalVersion extracts the pinned version from the build script line:
//
//	LIBSIGNAL_VERSION="${LIBSIGNAL_VERSION:-v0.x.y}"
func libsignalVersion(scriptPath string) (string, error) {
	b, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`LIBSIGNAL_VERSION="\$\{LIBSIGNAL_VERSION:-([^}]+)\}"`)
	m := re.FindSubmatch(b)
	if m == nil {
		return "", fmt.Errorf("LIBSIGNAL_VERSION line not found in %s", scriptPath)
	}
	return string(m[1]), nil
}

// platformNames maps GOOS/GOARCH to the asset naming used by the release workflow.
func platformNames() (string, string, error) {
	osMap := map[string]string{
		"linux":   "linux",
		"darwin":  "darwin",
		"windows": "windows",
	}
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
	}
	osName, ok := osMap[runtime.GOOS]
	if !ok {
		return "", "", fmt.Errorf("unsupported OS: %s (supported: linux, darwin, windows)", runtime.GOOS)
	}
	arch, ok := archMap[runtime.GOARCH]
	if !ok {
		return "", "", fmt.Errorf("unsupported arch: %s (supported: amd64, arm64)", runtime.GOARCH)
	}
	return osName, arch, nil
}

func downloadURL(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
