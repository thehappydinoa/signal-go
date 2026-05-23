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
		{useragent.DesktopLinux, "Signal-Desktop/8.10.0 Linux 6.1.0"},
		{useragent.DesktopMacOS, "Signal-Desktop/8.10.0 macOS 14.7.0"},
		{useragent.DesktopWindows, "Signal-Desktop/8.10.0 Windows 10"},
	}
	for _, tc := range tests {
		got := tc.profile.Format(useragent.Options{})
		if got != tc.want {
			t.Errorf("%s.Format() = %q, want %q", tc.profile, got, tc.want)
		}
	}
}

func TestUpstreamSourceMatchesFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		profile useragent.Profile
		wantSub []string
	}{
		{
			useragent.Android,
			[]string{"Signal-Android/", " Android/"},
		},
		{
			useragent.IOS,
			[]string{"Signal-iOS/", " iOS/"},
		},
		{
			useragent.DesktopLinux,
			[]string{"Signal-Desktop/", " Linux "},
		},
	}
	for _, tc := range cases {
		src, ok := tc.profile.UpstreamSource()
		if !ok {
			t.Fatalf("%s: missing upstream source", tc.profile)
		}
		if src.URL == "" || src.Repository == "" || src.File == "" {
			t.Fatalf("%s: incomplete source: %+v", tc.profile, src)
		}
		got := tc.profile.Format(useragent.Options{
			AppVersion: "1.2.3",
			OSVersion:  "99",
		})
		for _, sub := range tc.wantSub {
			if !strings.Contains(got, sub) {
				t.Errorf("%s formatted %q missing %q", tc.profile, got, sub)
			}
		}
		if !strings.Contains(got, "1.2.3") || !strings.Contains(got, "99") {
			t.Errorf("%s formatted %q missing injected versions", tc.profile, got)
		}
	}
}

func TestSignalGoHasNoUpstreamSource(t *testing.T) {
	t.Parallel()
	if _, ok := useragent.SignalGo.UpstreamSource(); ok {
		t.Fatal("signal-go should not claim an upstream source")
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
