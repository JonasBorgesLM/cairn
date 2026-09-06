package cairnhttp

import "html/template"

// interstitialData is what DefaultInterstitialTemplate (or a host's own
// template) receives. Destination is the raw destination text; html/template
// escapes it contextually wherever the template places it, which is what
// makes an XSS attempt in a destination render as text rather than markup.
type interstitialData struct {
	Destination string
}

// DefaultInterstitialTemplate is the page rendered for an Interstitial link
// when WithInterstitial is configured. It is deliberately plain and
// unstyled — a host is expected to replace it, and shipping something that
// looks finished would discourage that (ADR-0014).
//
// It loads no external resource: a warning page that pulls third-party
// script to say a third party is untrusted is a contradiction and a
// destination leak in itself. The continue control is a relative link
// ("?continue=1"), so it works unmodified regardless of where the handler is
// mounted, and it never carries a destination — the destination is re-read
// from the store on continuation (SR-24).
var DefaultInterstitialTemplate = template.Must(template.New("cairnhttp-interstitial").Parse(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Leaving this site</title></head>
<body>
<p>This link leads to:</p>
<pre>{{.Destination}}</pre>
<p><a href="?continue=1">Continue</a></p>
</body>
</html>
`))
