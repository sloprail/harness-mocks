package tools

import (
	"net/url"
	"strings"
)

// FetchLocalMessage is what a fetch of a local address is answered with: such a host is refused before
// any request, and the agent is pointed at the shell.
const FetchLocalMessage = "WebFetch cannot fetch localhost or other hostnames without a dot. To reach a local server, use Bash with curl instead."

// FetchableURL reports whether raw is a web address a fetch can be asked for: an http or https URL with
// a host.
func FetchableURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

// FetchRefused reports whether a fetch of the URL is refused before it is made: its host is localhost or
// any other name without a dot (a bare intranet name).
//
// sr:capability web-fetch-tool
func FetchRefused(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || !strings.Contains(host, ".")
}
