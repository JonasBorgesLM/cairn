package cairn

import (
	"net/url"
	"strconv"
	"strings"
)

// defaultPorts maps a scheme to the port RFC 3986 §6.2.3 treats as equivalent
// to no port at all.
var defaultPorts = map[string]string{
	"http":  "80",
	"https": "443",
}

// NormalizeURL applies only the equivalences RFC 3986 §6.2.2 declares safe,
// and nothing else (FR-16, ADR-0015). It does not sort query parameters,
// strip a trailing slash, lowercase the path, remove a "www." prefix, drop
// the fragment, or remove tracking parameters — each of those changes what a
// real server sees, and ADR-0015 records which server.
//
// NormalizeURL returns a new *url.URL; it does not modify u. Run it after
// [ParseDestination]'s syntax checks and before a [Policy] evaluates the
// result, so the policy always sees the canonical form.
func NormalizeURL(u *url.URL) *url.URL {
	out := *u

	out.Scheme = strings.ToLower(out.Scheme)
	out.Host = normalizeHost(out.Host, out.Scheme)

	if out.RawPath != "" || out.Path != "" || out.Host != "" {
		path := normalizePercentEscapes(out.EscapedPath())
		path = removeDotSegments(path)
		if path == "" && out.Host != "" {
			path = "/"
		}
		// path came from EscapedPath() and our own escaping logic, both of
		// which only ever produce valid percent-escapes, so decoding it
		// cannot fail; the fallback keeps the pre-normalization Path rather
		// than risk silently corrupting it if that invariant is ever wrong.
		if decoded, err := url.PathUnescape(path); err == nil {
			out.Path = decoded
		}
		out.RawPath = path
		if out.EscapedPath() == out.Path {
			// Go omits RawPath when it exactly matches the default encoding
			// of Path; keeping a redundant one would make later comparisons
			// see two URLs that both format identically as different values.
			out.RawPath = ""
		}
	}

	if out.RawQuery != "" {
		out.RawQuery = normalizePercentEscapes(out.RawQuery)
	}

	if out.Fragment != "" || out.RawFragment != "" {
		frag := normalizePercentEscapes(out.EscapedFragment())
		if decoded, err := url.PathUnescape(frag); err == nil {
			out.Fragment = decoded
		}
		out.RawFragment = frag
		if out.EscapedFragment() == out.Fragment {
			out.RawFragment = ""
		}
	}

	return &out
}

// normalizeHost lowercases host and removes a port that is the default for
// scheme (RFC 3986 §6.2.2.1, §6.2.3).
func normalizeHost(host, scheme string) string {
	if host == "" {
		return host
	}
	hostname, port, hasPort := splitHostPort(host)
	hostname = strings.ToLower(hostname)
	if hasPort && port == defaultPorts[scheme] {
		return hostname
	}
	if hasPort {
		return hostname + ":" + port
	}
	return hostname
}

// splitHostPort separates a URL authority's host and port without the strict
// validation net.SplitHostPort applies — u.Host is already known-good, having
// come from a successful url.Parse.
func splitHostPort(host string) (hostname, port string, hasPort bool) {
	if strings.HasPrefix(host, "[") {
		// IPv6 literal, e.g. "[::1]:8080" or "[::1]".
		end := strings.IndexByte(host, ']')
		if end < 0 {
			return host, "", false
		}
		hostname = host[:end+1]
		rest := host[end+1:]
		if strings.HasPrefix(rest, ":") {
			return hostname, rest[1:], true
		}
		return hostname, "", false
	}
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		return host[:i], host[i+1:], true
	}
	return host, "", false
}

// removeDotSegments implements the RFC 3986 §5.2.4 algorithm, applied to an
// already-percent-escaped path.
func removeDotSegments(path string) string {
	if path == "" {
		return path
	}

	var out []string
	rest := path
	for rest != "" {
		switch {
		case strings.HasPrefix(rest, "../"):
			rest = rest[3:]
		case strings.HasPrefix(rest, "./"):
			rest = rest[2:]
		case strings.HasPrefix(rest, "/./"):
			rest = "/" + rest[3:]
		case rest == "/.":
			rest = "/"
		case strings.HasPrefix(rest, "/../"):
			rest = "/" + rest[4:]
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		case rest == "/..":
			rest = "/"
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		case rest == ".", rest == "..":
			rest = ""
		default:
			// Move the first segment (including a leading "/", if any) from
			// rest to out.
			start := 1
			if !strings.HasPrefix(rest, "/") {
				start = 0
			}
			end := strings.IndexByte(rest[start:], '/')
			if end < 0 {
				out = append(out, rest)
				rest = ""
			} else {
				out = append(out, rest[:start+end])
				rest = rest[start+end:]
			}
		}
	}
	return strings.Join(out, "")
}

// normalizePercentEscapes decodes every percent-encoded unreserved character
// in an already-escaped string to its literal form, and uppercases the hex
// digits of every escape it leaves alone (RFC 3986 §6.2.2.2). It never
// touches an escape encoding a reserved character: doing so would turn an
// encoded separator into a literal one and change what the URL addresses.
func normalizePercentEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+2 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil {
			// Not a valid escape; leave the '%' as a literal byte, matching
			// what was already there rather than guessing at intent.
			b.WriteByte(s[i])
			continue
		}
		c := byte(v)
		if isUnreserved(c) {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteString(strings.ToUpper(s[i+1 : i+3]))
		}
		i += 2
	}
	return b.String()
}

// isUnreserved reports whether c is an RFC 3986 §2.3 unreserved character:
// ALPHA / DIGIT / "-" / "." / "_" / "~". These are the only bytes whose
// percent-encoding is safe to decode without changing the URL's meaning.
func isUnreserved(c byte) bool {
	switch {
	case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9':
		return true
	case c == '-' || c == '.' || c == '_' || c == '~':
		return true
	default:
		return false
	}
}
