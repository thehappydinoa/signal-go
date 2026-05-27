package profile

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionCloseNoPaths(t *testing.T) {
	s, err := Start("", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionWritesProfiles(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "mem.prof")
	cpu := filepath.Join(dir, "cpu.prof")

	s, err := Start(mem, cpu)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{mem, cpu} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if st.Size() == 0 {
			t.Errorf("%s is empty", p)
		}
	}
}

func TestBindRegistersFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	mem, cpu := Bind(fs)
	_ = fs.Parse([]string{"-memprofile=heap.prof", "-cpuprofile=cpu.prof"})
	if *mem != "heap.prof" || *cpu != "cpu.prof" {
		t.Fatalf("got mem=%q cpu=%q", *mem, *cpu)
	}
}
