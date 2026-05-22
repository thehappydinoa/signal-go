// Package useragent formats User-Agent strings that match official Signal
// clients.
//
// Each preset mirrors an upstream template documented by [Profile.UpstreamSource].
// Official definitions:
//
//   - Android: signalapp/Signal-Android StandardUserAgentInterceptor.java
//     "Signal-Android/" + VERSION_NAME + " Android/" + SDK_INT
//   - iOS: signalapp/Signal-iOS HttpHeaders.userAgentHeaderValueSignalIos
//     "Signal-iOS/{currentAppVersion} iOS/{systemVersion}"
//   - Desktop: signalapp/Signal-Desktop getUserAgent()
//     "Signal-Desktop/{appVersion} {platform} {os.release()}"
//
// signal-go sends the resolved string in both User-Agent and X-Signal-Agent.
// Upstream Desktop uses getUserAgent() for User-Agent and "OWD" for
// X-Signal-Agent; mobile clients set User-Agent only.
package useragent
