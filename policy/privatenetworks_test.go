package policy_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/policy"
)

// fakeResolver lets SR-07 be tested without a network: a test that needs DNS
// is a test that will be skipped.
type fakeResolver struct {
	addrs map[string][]netip.Addr
	err   error
}

func (r fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.addrs[host], nil
}

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("netip.ParseAddr(%q) error = %v", s, err)
	}
	return a
}

// --- Literal addresses in the host: each blocked range has its own case. ---

func TestBlockPrivateNetworks_BlocksLiteralAddresses(t *testing.T) {
	tests := []struct {
		name string
		host string
	}{
		{"IPv4 loopback", "127.0.0.1"},
		{"IPv6 loopback", "[::1]"},
		{"RFC 1918 10/8", "10.0.0.5"},
		{"RFC 1918 172.16/12", "172.16.0.5"},
		{"RFC 1918 192.168/16", "192.168.1.1"},
		{"link-local", "169.254.1.1"},
		{"cloud metadata address", "169.254.169.254"},
		{"IPv6 link-local", "[fe80::1]"},
		{"CGNAT 100.64.0.0/10", "100.64.0.1"},
		{"IPv4 unspecified", "0.0.0.0"},
		{"IPv6 unspecified", "[::]"},
		{"IPv4 multicast", "224.0.0.1"},
		{"IPv6 multicast", "[ff02::1]"},
		{"IPv6 unique-local", "[fc00::1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := policy.BlockPrivateNetworks(fakeResolver{})
			decision, reason := evaluate(t, p, "https://"+tt.host+"/")
			if decision != cairn.Deny {
				t.Fatalf("decision = %v, want Deny", decision)
			}
			if reason != cairn.ReasonPrivateAddress {
				t.Fatalf("reason = %q, want %q", reason, cairn.ReasonPrivateAddress)
			}
		})
	}
}

func TestBlockPrivateNetworks_AllowsAPublicLiteralAddress(t *testing.T) {
	p := policy.BlockPrivateNetworks(fakeResolver{})
	decision, _ := evaluate(t, p, "https://8.8.8.8/")
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

// --- IPv4-mapped IPv6: classified as its IPv4 equivalent (SR-07, issue #23). ---

func TestBlockPrivateNetworks_ClassifiesIPv4MappedIPv6AsItsIPv4Equivalent(t *testing.T) {
	tests := []struct {
		name string
		host string
		want cairn.Decision
	}{
		{"mapped loopback is blocked", "[::ffff:127.0.0.1]", cairn.Deny},
		{"mapped private is blocked", "[::ffff:10.0.0.1]", cairn.Deny},
		{"mapped public is allowed", "[::ffff:8.8.8.8]", cairn.Allow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := policy.BlockPrivateNetworks(fakeResolver{})
			decision, _ := evaluate(t, p, "https://"+tt.host+"/")
			if decision != tt.want {
				t.Fatalf("decision = %v, want %v", decision, tt.want)
			}
		})
	}
}

// --- Non-decimal IPv4 literals: rejected outright (SR-07, issue #23). ---
//
// Negative control: each of these was confirmed to be classified as an
// ordinary hostname -- falling through to DNS resolution instead of being
// rejected outright -- when the ambiguous-literal check was temporarily
// disabled. Recorded next to TestBlockPrivateNetworks_ambiguousIPv4 below,
// which is the check that was disabled.

func TestBlockPrivateNetworks_RejectsNonDecimalIPv4Forms(t *testing.T) {
	tests := []struct {
		name string
		host string
	}{
		{"hex", "0x7f.0.0.1"},
		{"octal", "0177.0.0.1"},
		{"decimal integer", "2130706433"},
		{"mixed shorthand", "127.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No resolver entry for these hosts: if the ambiguous-literal
			// check did not fire, this would fall through to a DNS lookup
			// that fakeResolver answers with no addresses, which would Allow
			// -- proving the rejection happens before resolution, not after.
			p := policy.BlockPrivateNetworks(fakeResolver{})
			decision, reason := evaluate(t, p, "https://"+tt.host+"/")
			if decision != cairn.Deny {
				t.Fatalf("decision = %v, want Deny", decision)
			}
			if reason != cairn.ReasonPrivateAddress {
				t.Fatalf("reason = %q, want %q", reason, cairn.ReasonPrivateAddress)
			}
		})
	}
}

func TestBlockPrivateNetworks_AllowsAnOrdinaryDottedDecimalIPv4(t *testing.T) {
	// The one numeric-looking form that is not ambiguous: a plain, complete
	// dotted-decimal quad with no leading zeros -- exactly what
	// netip.ParseAddr itself accepts.
	p := policy.BlockPrivateNetworks(fakeResolver{})
	decision, _ := evaluate(t, p, "https://93.184.216.34/")
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

// --- Hostnames: resolved through the injected Resolver. ---

func TestBlockPrivateNetworks_ResolvesAHostnameAndBlocksAPrivateResult(t *testing.T) {
	p := policy.BlockPrivateNetworks(fakeResolver{
		addrs: map[string][]netip.Addr{"internal.example": {mustAddr(t, "10.0.0.5")}},
	})
	decision, reason := evaluate(t, p, "https://internal.example/")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
	if reason != cairn.ReasonPrivateAddress {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonPrivateAddress)
	}
}

func TestBlockPrivateNetworks_ResolvesAHostnameAndAllowsAPublicResult(t *testing.T) {
	p := policy.BlockPrivateNetworks(fakeResolver{
		addrs: map[string][]netip.Addr{"public.example": {mustAddr(t, "93.184.216.34")}},
	})
	decision, _ := evaluate(t, p, "https://public.example/")
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

func TestBlockPrivateNetworks_BlocksIfAnyResolvedAddressIsPrivate(t *testing.T) {
	// A multi-homed name where only one of several answers is private must
	// still be blocked -- checking only the first address would be a bypass.
	p := policy.BlockPrivateNetworks(fakeResolver{
		addrs: map[string][]netip.Addr{
			"multihomed.example": {mustAddr(t, "93.184.216.34"), mustAddr(t, "10.0.0.5")},
		},
	})
	decision, _ := evaluate(t, p, "https://multihomed.example/")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
}

// A resolver error must produce Deny, never Allow (SR-20).
//
// Negative control: this test was run against an implementation that
// returned Allow on a resolver error, and it failed as expected. Recorded
// here; the implementation was corrected immediately after.
func TestBlockPrivateNetworks_ResolverErrorProducesDenyNeverAllow(t *testing.T) {
	p := policy.BlockPrivateNetworks(fakeResolver{err: errors.New("no such host")})
	decision, reason := evaluate(t, p, "https://unresolvable.example/")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny (fail-closed, SR-20)", decision)
	}
	if reason != cairn.ReasonResolveFailed {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonResolveFailed)
	}
}
