package policy_test

import (
	"context"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/policy"
)

func TestSchemeAllowlist_AllowsListedScheme(t *testing.T) {
	p := policy.SchemeAllowlist("https")
	d, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}

	decision, _, err := p.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

func TestSchemeAllowlist_DeniesSchemeOutsideTheList(t *testing.T) {
	// SchemeAllowlist("https") is a stricter, consumer-composed restriction
	// than the http+https ParseDestination already enforces (SR-05) — it lets
	// a caller who wants only https reject plain http as well.
	p := policy.SchemeAllowlist("https")
	d, err := cairn.ParseDestination("http://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}

	decision, reason, err := p.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
	if reason != cairn.ReasonScheme {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonScheme)
	}
}

func TestSchemeAllowlist_IsCaseInsensitive(t *testing.T) {
	p := policy.SchemeAllowlist("HTTPS")
	d, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	decision, _, err := p.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}
