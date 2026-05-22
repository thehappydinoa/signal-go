package useragent_test

import (
	"strings"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/web/useragent"
)

func TestProfileFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		profile useragent.Profile
		want    string
	}{
		{useragent.SignalGo, "signal-go"},
		{useragent.Android, "Signal-Android/8.12.1 Android/35"},
		{useragent.IOS, "Signal-iOS/8.13 iOS/18.2"},
		{useragent.DesktopLinux, "Signal-Desktop/7.47.0 Linux 6.1.0"},
		{useragent.DesktopMacOS, "Signal-Desktop/7.47.0 macOS 14.7.0"},
		{useragent.DesktopWindows, "Signal-Desktop/7.47.0 Windows 10"},
	}
	for _, tc := range tests {
		got := tc.profile.Format(useragent.Options{})
		if got != tc.want {
			t.Errorf("%s.Format() = %q, want %q", tc.profile, got, tc.want)
		}
	}
}

func TestResolveOverrideWins(t *testing.T) {
	t.Parallel()
	got := useragent.Resolve(useragent.Android, "custom-agent/1.0", useragent.Options{})
	if got != "custom-agent/1.0" {
		t.Fatalf("override: got %q", got)
	}
}

func TestParseAliases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want useragent.Profile
	}{
		{"desktop-linux", useragent.DesktopLinux},
		{"linux", useragent.DesktopLinux},
		{"macos", useragent.DesktopMacOS},
		{"win", useragent.DesktopWindows},
	} {
		got, err := useragent.Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("Parse(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseUnknown(t *testing.T) {
	t.Parallel()
	if _, err := useragent.Parse("telegram"); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestFormatCustomVersions(t *testing.T) {
	t.Parallel()
	got := useragent.Android.Format(useragent.Options{
		AppVersion: "9.0.0",
		OSVersion:  "36",
	})
	if !strings.Contains(got, "9.0.0") || !strings.Contains(got, "36") {
		t.Fatalf("custom versions not applied: %q", got)
	}
}
