package signal

import "github.com/thehappydinoa/signal-go/internal/web/useragent"

// UserAgentProfile selects a realistic Signal client User-Agent preset.
// See [UserAgentProfileUpstreamSource] for official upstream citations.
type UserAgentProfile = useragent.Profile

// User-agent profile presets matching upstream Signal clients.
const (
	UserAgentSignalGo       = useragent.SignalGo
	UserAgentAndroid        = useragent.Android
	UserAgentIOS            = useragent.IOS
	UserAgentDesktopLinux   = useragent.DesktopLinux
	UserAgentDesktopMacOS   = useragent.DesktopMacOS
	UserAgentDesktopWindows = useragent.DesktopWindows
)

// UserAgentOptions overrides app/OS version strings in a profile.
type UserAgentOptions = useragent.Options

// ParseUserAgentProfile parses a profile name (e.g. "desktop-linux").
func ParseUserAgentProfile(s string) (UserAgentProfile, error) {
	return useragent.Parse(s)
}

// UserAgentProfiles returns supported preset names.
func UserAgentProfiles() []UserAgentProfile {
	return useragent.Profiles()
}

// UserAgentUpstreamSource cites the official Signal client code for profile.
// ok is false for [UserAgentSignalGo].
func UserAgentUpstreamSource(profile UserAgentProfile) (useragent.UpstreamSource, bool) {
	return profile.UpstreamSource()
}

func resolveUserAgent(profile UserAgentProfile, override string, opts UserAgentOptions) string {
	return useragent.Resolve(profile, override, opts)
}
