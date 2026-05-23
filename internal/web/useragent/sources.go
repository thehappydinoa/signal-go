package useragent

// UpstreamSource cites the official Signal client code a [Profile] mimics.
type UpstreamSource struct {
	// Repository is the canonical GitHub org/repo.
	Repository string
	// File is the path within that repository.
	File string
	// Line is the defining line or small range in upstream.
	Line string
	// Format is the upstream template. Placeholders:
	//   {appVersion} — app release string
	//   {osVersion}  — Android SDK int, iOS systemVersion, or Desktop os.release()
	//   {platform}   — Desktop only: "Linux", "macOS", or "Windows"
	Format string
	// URL links to the upstream definition on GitHub.
	URL string
	// Notes describes header usage differences from signal-go.
	Notes string
}

// UpstreamSource returns the official upstream citation for p. [SignalGo]
// has no upstream counterpart.
func (p Profile) UpstreamSource() (UpstreamSource, bool) {
	switch p {
	case Android:
		return UpstreamSource{
			Repository: "signalapp/Signal-Android",
			File:       "app/src/main/java/org/thoughtcrime/securesms/net/StandardUserAgentInterceptor.java",
			Line:       "12",
			Format:     "Signal-Android/{appVersion} Android/{osVersion}",
			URL:        "https://github.com/signalapp/Signal-Android/blob/main/app/src/main/java/org/thoughtcrime/securesms/net/StandardUserAgentInterceptor.java#L12",
			Notes: "Upstream sets the HTTP User-Agent header only (via UserAgentInterceptor). " +
				"appVersion is BuildConfig.VERSION_NAME; osVersion is Build.VERSION.SDK_INT (integer API level, not a marketing release like \"15\").",
		}, true
	case IOS:
		return UpstreamSource{
			Repository: "signalapp/Signal-iOS",
			File:       "SignalServiceKit/Network/HttpHeaders.swift",
			Line:       "151-153",
			Format:     "Signal-iOS/{appVersion} iOS/{osVersion}",
			URL:        "https://github.com/signalapp/Signal-iOS/blob/main/SignalServiceKit/Network/HttpHeaders.swift#L151-L153",
			Notes: "Upstream sets the HTTP User-Agent header in addDefaultHeaders(). " +
				"appVersion is AppVersionImpl.shared.currentAppVersion; osVersion is UIDevice.current.systemVersion.",
		}, true
	case DesktopLinux:
		return desktopUpstreamSource("Linux", "linux")
	case DesktopMacOS:
		return desktopUpstreamSource("macOS", "darwin")
	case DesktopWindows:
		return desktopUpstreamSource("Windows", "win32")
	default:
		return UpstreamSource{}, false
	}
}

func desktopUpstreamSource(platformLabel, platformKey string) (UpstreamSource, bool) {
	return UpstreamSource{
		Repository: "signalapp/Signal-Desktop",
		File:       "ts/util/getUserAgent.ts",
		Line:       "7-11, 18-28",
		Format:     "Signal-Desktop/{appVersion} {platform} {osVersion}",
		URL:        "https://github.com/signalapp/Signal-Desktop/blob/main/ts/util/getUserAgent.ts#L7-L28",
		Notes: "Upstream getUserAgent(appVersion, release) maps process.platform " +
			"\"" + platformKey + "\" → \"" + platformLabel + "\" and appends os.release(). " +
			"REST/WebSocket User-Agent uses this value (see ts/textsecure/WebAPI.ts). " +
			"X-Signal-Agent is separately hard-coded to \"OWD\" on Desktop; signal-go currently sends the same string in both headers.",
	}, true
}

// DefaultVersionSource notes where each default snapshot version was read
// from upstream at the time these presets were authored.
func DefaultVersionSource(profile Profile) (repo, file, url string, ok bool) {
	switch profile {
	case Android:
		return "signalapp/Signal-Android",
			"app/build.gradle.kts (canonicalVersionName)",
			"https://github.com/signalapp/Signal-Android/blob/main/app/build.gradle.kts",
			true
	case IOS:
		return "signalapp/Signal-iOS",
			"Signal/Signal-Info.plist (CFBundleShortVersionString)",
			"https://github.com/signalapp/Signal-iOS/blob/main/Signal/Signal-Info.plist",
			true
	case DesktopLinux, DesktopMacOS, DesktopWindows:
		return "signalapp/Signal-Desktop",
			"package.json (version field on a release tag)",
			"https://github.com/signalapp/Signal-Desktop/blob/v8.10.0/package.json#L3",
			true
	default:
		return "", "", "", false
	}
}
