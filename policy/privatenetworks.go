package policy

import (
	"context"
	"net/netip"
	"slices"
	"strings"

	"github.com/JonasBorgesLM/cairn"
)

// Resolver looks up the network addresses a host resolves to. It exists so
// SR-07 is testable without a network — a test that needs DNS is a test that
// will be skipped — and net.DefaultResolver satisfies it directly, since its
// LookupNetIP method has exactly this signature.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// cgnat is the shared-address-space range RFC 6598 assigns for carrier-grade
// NAT (100.64.0.0/10). It has no equivalent among netip.Addr's built-in
// classification methods.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// blockPrivateNetworks is a cairn.Policy rejecting destinations whose host is,
// or resolves to, a loopback, private, link-local, CGNAT, unspecified or
// multicast address (SR-07, T-05).
type blockPrivateNetworks struct {
	resolver Resolver
}

// BlockPrivateNetworks returns a Policy blocking loopback, RFC 1918,
// link-local (including 169.254.169.254 and fe80::/10), CGNAT
// (100.64.0.0/10), unspecified, multicast, and IPv6 unique-local addresses,
// whether the host is a literal address or a name that resolves to one via r
// (SR-07, T-05).
//
// A resolver error returns Deny with [cairn.ReasonResolveFailed], never
// Allow: failing open here would make DNS unavailability a temporary
// permission to shorten an internal address (SR-20).
//
// # What this does not do
//
// cairn never fetches a destination, so this protects nothing of cairn's own.
// It protects third-party services that follow redirects: without it, an
// abuser turns an allowlisted short domain into a bypass into another
// service's own metadata endpoint (T-05).
//
// It also binds a hostname to an address at create time. DNS may answer
// differently when the link is later resolved. No create-time check can close
// that gap; the complete mitigation belongs to the service that fetches the
// destination, which must validate the address it actually connects to.
// BlockPrivateNetworks raises the cost of the attack and removes the trivial
// literal-address case. It does not remove the class.
func BlockPrivateNetworks(r Resolver) cairn.Policy {
	return blockPrivateNetworks{resolver: r}
}

// Evaluate implements cairn.Policy.
func (p blockPrivateNetworks) Evaluate(ctx context.Context, d cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	host := d.Host()

	if addr, err := netip.ParseAddr(host); err == nil {
		if isBlockedAddr(addr) {
			return cairn.Deny, cairn.ReasonPrivateAddress, nil
		}
		return cairn.Allow, "", nil
	}

	// host did not parse as a literal address. Before treating it as a name
	// to resolve, check whether it is instead an address written in a form
	// netip.ParseAddr deliberately refuses -- hex, octal, a single decimal
	// integer, or a shorthand grouping -- which some HTTP clients accept as a
	// literal IP even though cairn's own parser does not. Refusing it here
	// closes the disagreement between the two parsers rather than resolving
	// a name that was never really a name (SR-07).
	if looksLikeAmbiguousIPv4(host) {
		return cairn.Deny, cairn.ReasonPrivateAddress, nil
	}

	addrs, err := p.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		//nolint:nilerr // deliberate: ADR-0005 classifies a resolver failure as Deny+ReasonResolveFailed, not a Go error (SR-20).
		return cairn.Deny, cairn.ReasonResolveFailed, nil
	}
	if slices.ContainsFunc(addrs, isBlockedAddr) {
		return cairn.Deny, cairn.ReasonPrivateAddress, nil
	}
	return cairn.Allow, "", nil
}

// isBlockedAddr reports whether addr falls in a range SR-07 blocks. An
// IPv4-mapped IPv6 address (::ffff:0:0/96) is unmapped to its IPv4 equivalent
// first, so it is classified exactly as that address would be — not merely
// exempted from every IPv4-shaped check because it is nominally an IPv6
// value (issue #23).
func isBlockedAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	switch {
	case addr.IsLoopback():
		return true
	case addr.IsPrivate(): // RFC 1918 (IPv4) and RFC 4193 (IPv6 ULA, fc00::/7)
		return true
	case addr.IsLinkLocalUnicast(): // 169.254.0.0/16 and fe80::/10
		return true
	case addr.IsLinkLocalMulticast():
		return true
	case addr.IsUnspecified():
		return true
	case addr.IsMulticast():
		return true
	case cgnat.Contains(addr):
		return true
	default:
		return false
	}
}

// looksLikeAmbiguousIPv4 reports whether host resembles an attempt to write
// an IPv4 address in a form netip.ParseAddr refuses — hexadecimal, octal, a
// single decimal integer standing for the whole 32-bit address, or a
// shorthand grouping of fewer than four parts — any of which some HTTP
// clients and libraries still parse as a literal address. cairn refuses these
// outright rather than resolving them as hostnames: DNS could answer for such
// a name (an attacker can register one), which would only move the
// disagreement from "does this parse as an IP" to "does this resolve
// publicly", and not close it (SR-07).
//
// A host is left alone — treated as an ordinary name — the moment any part is
// not entirely digits, or a "0x"/"0X" hex escape, or a "0"-prefixed run of
// digits: that is what keeps a real hostname label like "example" or "a1b2"
// from being misclassified.
func looksLikeAmbiguousIPv4(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if !looksNumeric(part) {
			return false
		}
	}
	return true
}

// looksNumeric reports whether part is entirely digits, a "0x"/"0X" hex
// escape followed by hex digits, or a "0"-prefixed run of digits (octal-
// shaped). All three are numeric-literal forms some URL/IP parsers accept;
// none of them is a form netip.ParseAddr accepts unescaped, which is exactly
// why reaching this function means ParseAddr already failed.
func looksNumeric(part string) bool {
	if part == "" {
		return false
	}
	if len(part) > 2 && (part[:2] == "0x" || part[:2] == "0X") {
		return isAllHexDigits(part[2:])
	}
	return isAllDecimalDigits(part)
}

func isAllDecimalDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isAllHexDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
