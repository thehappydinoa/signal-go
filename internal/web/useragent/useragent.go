// Package useragent formats realistic Signal client User-Agent strings.
//
// signal-go sends the same value in User-Agent and X-Signal-Agent (see
// [github.com/thehappydinoa/signal-go/internal/web]). Upstream mobile
// clients only set User-Agent; Signal Desktop sets User-Agent to
// getUserAgent() and X-Signal-Agent to "OWD". We mimic the User-Agent
// formats from Signal-Android, Signal-iOS, and Signal-Desktop.
package useragent

import (
	"fmt"
	"strings"
)

// Profile names a preset client identity.
type Profile string

const (
	// SignalGo is the honest development default.
	SignalGo Profile = "signal-go"
	// Android mimics Signal-Android's StandardUserAgentInterceptor.
	Android Profile = "android"
	// IOS mimics Signal-iOS HttpHeaders.userAgentHeaderValueSignalIos.
	IOS Profile = "ios"
	// DesktopLinux mimics Signal-Desktop on Linux linked devices.
	DesktopLinux Profile = "desktop-linux"
	// DesktopMacOS mimics Signal-Desktop on macOS linked devices.
	DesktopMacOS Profile = "desktop-macos"
	// DesktopWindows mimics Signal-Desktop on Windows linked devices.
	DesktopWindows Profile = "desktop-windows"
)

// Snapshot versions aligned with upstream clients near libsignal v0.94.1.
// Bump intentionally when we refresh the libsignal pin.
const (
	DefaultAndroidAppVersion  = "8.12.1"
	DefaultAndroidSDK         = "35"
	DefaultIOSAppVersion      = "8.13"
	DefaultIOSSystemVersion   = "18.2"
	DefaultDesktopAppVersion  = "7.47.0"
	DefaultLinuxKernelRelease = "6.1.0"
	DefaultMacOSRelease       = "14.7.0"
	DefaultWindowsRelease     = "10"
)

// Options overrides preset version strings.
type Options struct {
	AppVersion string
	OSVersion  string
}

// Resolve returns override when set; otherwise formats profile (default
// [SignalGo] when profile is empty).
func Resolve(profile Profile, override string, opts Options) string {
	if override != "" {
		return override
	}
	if profile == "" {
		profile = SignalGo
	}
	return profile.Format(opts)
}

// Parse returns a [Profile] for a CLI/config string.
func Parse(s string) (Profile, error) {
	switch Profile(strings.ToLower(strings.TrimSpace(s))) {
	case "", SignalGo:
		return SignalGo, nil
	case Android:
		return Android, nil
	case IOS, "iphone", "ipad":
		return IOS, nil
	case DesktopLinux, "desktop", "linux":
		return DesktopLinux, nil
	case DesktopMacOS, "macos", "darwin":
		return DesktopMacOS, nil
	case DesktopWindows, "windows", "win":
		return DesktopWindows, nil
	default:
		return "", fmt.Errorf("useragent.Parse: unknown profile %q", s)
	}
}

// Format renders the User-Agent string for p.
func (p Profile) Format(opts Options) string {
	switch p {
	case SignalGo, "":
		return string(SignalGo)
	case Android:
		ver := opts.AppVersion
		if ver == "" {
			ver = DefaultAndroidAppVersion
		}
		sdk := opts.OSVersion
		if sdk == "" {
			sdk = DefaultAndroidSDK
		}
		return fmt.Sprintf("Signal-Android/%s Android/%s", ver, sdk)
	case IOS:
		ver := opts.AppVersion
		if ver == "" {
			ver = DefaultIOSAppVersion
		}
		sys := opts.OSVersion
		if sys == "" {
			sys = DefaultIOSSystemVersion
		}
		return fmt.Sprintf("Signal-iOS/%s iOS/%s", ver, sys)
	case DesktopLinux:
		return desktopFormat("Linux", opts, DefaultLinuxKernelRelease)
	case DesktopMacOS:
		return desktopFormat("macOS", opts, DefaultMacOSRelease)
	case DesktopWindows:
		return desktopFormat("Windows", opts, DefaultWindowsRelease)
	default:
		return string(SignalGo)
	}
}

func desktopFormat(platform string, opts Options, defaultRelease string) string {
	ver := opts.AppVersion
	if ver == "" {
		ver = DefaultDesktopAppVersion
	}
	release := opts.OSVersion
	if release == "" {
		release = defaultRelease
	}
	return fmt.Sprintf("Signal-Desktop/%s %s %s", ver, platform, release)
}

// Profiles lists supported preset names in display order.
func Profiles() []Profile {
	return []Profile{
		SignalGo,
		Android,
		IOS,
		DesktopLinux,
		DesktopMacOS,
		DesktopWindows,
	}
}
