package policy_test

import (
	"context"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/policy"
)

func evaluate(t *testing.T, p cairn.Policy, raw string) (cairn.Decision, cairn.RejectReason) {
	t.Helper()
	d, err := cairn.ParseDestination(raw)
	if err != nil {
		t.Fatalf("ParseDestination(%q) error = %v", raw, err)
	}
	decision, reason, err := p.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	return decision, reason
}

func TestBlockOwnDomains_DeniesTheExactDomain(t *testing.T) {
	p := policy.BlockOwnDomains("example.com")
	decision, reason := evaluate(t, p, "https://example.com/")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
	if reason != cairn.ReasonOwnDomain {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonOwnDomain)
	}
}

func TestBlockOwnDomains_DeniesASubdomain(t *testing.T) {
	p := policy.BlockOwnDomains("example.com")
	decision, _ := evaluate(t, p, "https://a.example.com/")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
}

// The exact case the subdomain matcher must not get wrong: a domain that
// merely ends with the same characters is not a subdomain.
func TestBlockOwnDomains_DoesNotMatchALookalikeDomain(t *testing.T) {
	p := policy.BlockOwnDomains("example.com")
	decision, _ := evaluate(t, p, "https://notexample.com/")
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow (notexample.com is not a subdomain of example.com)", decision)
	}
}

func TestBlockOwnDomains_IsCaseInsensitive(t *testing.T) {
	p := policy.BlockOwnDomains("EXAMPLE.com")
	decision, _ := evaluate(t, p, "https://example.com/")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
}

func TestBlockOwnDomains_HandlesATrailingDotOnTheHost(t *testing.T) {
	p := policy.BlockOwnDomains("example.com")
	decision, _ := evaluate(t, p, "https://example.com./")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
}

func TestBlockOwnDomains_AllowsAnUnrelatedDomain(t *testing.T) {
	p := policy.BlockOwnDomains("example.com")
	decision, _ := evaluate(t, p, "https://other.example/")
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}
