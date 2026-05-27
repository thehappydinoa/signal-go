package cliargs

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreFromFlagsAbs(t *testing.T) {
	dir := t.TempDir()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	storeDir, passFile, plain := StoreBind(fs, ".")
	*storeDir = dir
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	s, err := StoreFromFlags(storeDir, passFile, plain)
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	if s.Dir != abs {
		t.Errorf("Dir = %q, want %q", s.Dir, abs)
	}
}

func TestReadPassphraseFromFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(f, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPassphrase(f)
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret" {
		t.Errorf("got %q, want secret", got)
	}
}

func TestClientFromFlagsInvalid(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	preset, ua := ClientBind(fs)
	*preset = "not-a-real-preset"
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ClientFromFlags(preset, ua); err == nil {
		t.Fatal("expected error for invalid -client")
	}
}

func TestSubcommand(t *testing.T) {
	cmd, rest, ok := Subcommand([]string{"link", "-store", "x"})
	if !ok || cmd != "link" || len(rest) != 2 {
		t.Fatalf("got cmd=%q rest=%v ok=%v", cmd, rest, ok)
	}
	_, _, ok = Subcommand(nil)
	if ok {
		t.Fatal("expected !ok for empty argv")
	}
}
