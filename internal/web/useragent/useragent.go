package useragent

import (
	"fmt"
	"strings"
)

// Profile names a preset client identity.
type Profile string

const (
	// SignalGo is the honest development default (no upstream counterpart).
	SignalGo Profile = "signal-go"
	// Android matches Signal-Android StandardUserAgentInterceptor.USER_AGENT.
	// See [Profile.UpstreamSource].
	Android Profile = "android"
	// IOS matches Signal-iOS HttpHeaders.userAgentHeaderValueSignalIos.
	// See [Profile.UpstreamSource].
	IOS Profile = "ios"
	// DesktopLinux matches Signal-Desktop getUserAgent on process.platform "linux".
	DesktopLinux Profile = "desktop-linux"
	// DesktopMacOS matches Signal-Desktop getUserAgent on process.platform "darwin".
	DesktopMacOS Profile = "desktop-macos"
	// DesktopWindows matches Signal-Desktop getUserAgent on process.platform "win32".
	DesktopWindows Profile = "desktop-windows"
)

// Default snapshot versions for presets. These are examples only — upstream
// reads live values from BuildConfig, CFBundleShortVersionString, and
// package.json. See [DefaultVersionSource] for where each default was taken.
const (
	DefaultAndroidAppVersion  = "8.12.1" // Signal-Android canonicalVersionName at authoring time
	DefaultAndroidSDK         = "35"     // example API level (Android 15)
	DefaultIOSAppVersion      = "8.13"   // Signal-iOS CFBundleShortVersionString at authoring time
	DefaultIOSSystemVersion   = "18.2"   // example UIDevice.systemVersion
	DefaultDesktopAppVersion  = "8.10.0" // Signal-Desktop v8.10.0 release tag (stale → HTTP 499)
	DefaultLinuxKernelRelease = "6.1.0"  // example os.release() prefix on Linux
	DefaultMacOSRelease       = "14.7.0" // example os.release() on macOS
	DefaultWindowsRelease     = "10"     // example os.release() major on Windows
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
