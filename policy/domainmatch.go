package policy

import "strings"

// hostMatchesDomain reports whether host is domain itself or a subdomain of
// it — "a.example.com" matches "example.com", but "notexample.com" does not,
// because it merely ends with the same characters rather than being a
// dot-separated descendant. Comparison is case-insensitive, and a trailing
// dot on either side (the absolute-FQDN form) is ignored.
func hostMatchesDomain(host, domain string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if domain == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}
