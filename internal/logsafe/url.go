// Package logsafe contains helpers for converting sensitive values into log-safe representations.
package logsafe

import "net/url"

// URL returns a URL string that is safe to write to logs.
//
// It preserves the scheme, host, port, and path because those identify the
// endpoint operators need for debugging, but strips userinfo, query strings,
// and fragments because they commonly contain passwords, API keys, or tokens.
// Invalid or hostless URLs are replaced with a placeholder to avoid echoing a
// malformed secret-bearing value back into logs.
func URL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "<invalid-url>"
	}

	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// URLs returns log-safe representations for each URL in rawURLs.
//
// The returned slice is a new allocation so callers can log or modify it
// without changing the original configuration values used for connections.
func URLs(rawURLs []string) []string {
	redacted := make([]string, 0, len(rawURLs))
	for _, raw := range rawURLs {
		redacted = append(redacted, URL(raw))
	}
	return redacted
}
