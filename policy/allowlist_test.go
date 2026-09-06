package policy_test

import (
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/policy"
)

func TestAllowlist_AllowsAListedDomain(t *testing.T) {
	p := policy.Allowlist("example.com")
	decision, _ := evaluate(t, p, "https://example.com/")
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

func TestAllowlist_AllowsASubdomainOfAListedDomain(t *testing.T) {
	p := policy.Allowlist("example.com")
	decision, _ := evaluate(t, p, "https://a.example.com/")
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

// FR-13: non-members get a warning, not a rejection -- that is what makes
// ADR-0014's interstitial page possible without a second classification pass.
func TestAllowlist_NonMemberIsInterstitialNotDeny(t *testing.T) {
	p := policy.Allowlist("example.com")
	decision, reason := evaluate(t, p, "https://other.example/")
	if decision != cairn.Interstitial {
		t.Fatalf("decision = %v, want Interstitial", decision)
	}
	if reason != cairn.ReasonNotAllowlisted {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonNotAllowlisted)
	}
}

// An empty allowlist means everything is Interstitial. This is intentional,
// not a footgun: a policy with no members to allow has nothing to Allow.
func TestAllowlist_EmptyListMeansEverythingIsInterstitial(t *testing.T) {
	p := policy.Allowlist()
	decision, _ := evaluate(t, p, "https://example.com/")
	if decision != cairn.Interstitial {
		t.Fatalf("decision = %v, want Interstitial", decision)
	}
}
