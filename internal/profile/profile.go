// Package profile helps long-running CLIs write Go runtime profiles on exit.
// Flags mirror go test's -memprofile / -cpuprofile but apply to normal binaries.
package profile

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/pprof"
)

// Bind registers -memprofile and -cpuprofile on fs (written on process exit).
func Bind(fs *flag.FlagSet) (memProfile, cpuProfile *string) {
	memProfile = fs.String("memprofile", "", "write heap profile to this file on exit (Ctrl+C or normal return)")
	cpuProfile = fs.String("cpuprofile", "", "write CPU profile to this file on exit")
	return memProfile, cpuProfile
}

// Session records CPU profiling for the process lifetime. Call [Session.Close]
// once before exit (typically defer) to flush profiles.
type Session struct {
	memPath string
	stopCPU func()
}

// Start begins CPU profiling when cpuPath is non-empty.
func Start(memPath, cpuPath string) (*Session, error) {
	s := &Session{memPath: memPath}
	if cpuPath == "" {
		s.stopCPU = func() {}
		return s, nil
	}
	f, err := os.Create(cpuPath)
	if err != nil {
		return nil, fmt.Errorf("profile: create cpu profile: %w", err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("profile: start cpu profile: %w", err)
	}
	s.stopCPU = func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}
	return s, nil
}

// Close stops CPU profiling (if any) and writes the heap profile.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	if s.stopCPU != nil {
		s.stopCPU()
		s.stopCPU = nil
	}
	if s.memPath == "" {
		return nil
	}
	f, err := os.Create(s.memPath)
	if err != nil {
		return fmt.Errorf("profile: create mem profile: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := pprof.WriteHeapProfile(f); err != nil {
		return fmt.Errorf("profile: write heap profile: %w", err)
	}
	return nil
}

// PathsFromFlags returns paths from Bind's pointers after flag.Parse.
func PathsFromFlags(memProfile, cpuProfile *string) (mem, cpu string, err error) {
	if memProfile == nil || cpuProfile == nil {
		return "", "", errors.New("profile: nil flag pointer")
	}
	return *memProfile, *cpuProfile, nil
}
