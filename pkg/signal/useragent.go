package signal

import "github.com/thehappydinoa/signal-go/internal/web/useragent"

// UserAgentProfile selects a realistic Signal client User-Agent preset.
// Used when [LinkOptions.UserAgent], [OpenOptions.UserAgent], or
// [bot.Options.UserAgent] is empty.
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

func resolveUserAgent(profile UserAgentProfile, override string, opts UserAgentOptions) string {
	return useragent.Resolve(profile, override, opts)
}
